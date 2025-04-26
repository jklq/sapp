package category

import (
	"fmt" // Import fmt for error formatting
	"log/slog"
	"time"
)

func (w CategorizingPool) jobDbPoller(errCh chan<- error) {
	for {
		time.Sleep(2 * time.Second)

		rows, err := w.db.Query(`SELECT id, status, prompt, buyer, shared_mode, shared_with, total_amount
		FROM ai_categorization_jobs WHERE status="queued"`)

		if err != nil {
			slog.Error("polling error", "error", err)
			errCh <- err
			continue
		}

		var jobs []Job = make([]Job, 0)
		for rows.Next() {
			var job Job = Job{}

			err := rows.Scan(&job.Id, &job.Status, &job.Prompt, &job.Buyer, &job.SharedMode, &job.SharedWithId, &job.TotalAmount)
			if err != nil {
				slog.Error("polling error", "error", err)

				rows.Close()
				errCh <- err
				continue
			}
			jobs = append(jobs, job)
		}
		for _, job := range jobs {
			w.unhandledJobs <- job
		}
	}
}

// TODO: error handling if fails mid-job
func (p CategorizingPool) worker(jobCh <-chan Job, errCh chan<- error) {
	for job := range jobCh {
		// TODO: oppdater jobbens status i DB til processing
		var buyerName string
		row := p.db.QueryRow("SELECT first_name FROM users WHERE id = ?", job.Buyer)
		err := row.Scan(&buyerName)
		if err != nil {
			slog.Error("error when scanning first name from buyer", "error", err, "id", job.Buyer)
			errCh <- err
			continue
		}

		var sharedWith *Person = nil
		if job.SharedMode != "alone" {
			if job.SharedWithId == nil {
				slog.Error("share mode is not alone, but sharedWith id is nil")
				errCh <- err
				continue
			}

			sharedWith = &Person{Id: *job.SharedWithId}
			row := p.db.QueryRow("SELECT first_name FROM users WHERE id = ?", job.SharedWithId)
			err = row.Scan(&sharedWith.Name)
			if err != nil {
				slog.Error("failed when getting first name of sharedWith", "error", err)
				errCh <- err
				continue
			}
		}

		_, err = p.db.Exec("UPDATE ai_categorization_jobs SET status = \"processing\", status_updated_at=? WHERE id = ?", time.Now().UTC(), job.Id)

		if err != nil {
			slog.Error("failed when setting status to processing", "error", err)
			errCh <- err
			continue
		}

		// Pass the db connection (p.db) to ProcessCategorizationJob
		result, err := ProcessCategorizationJob(p.db, CategorizationParams{
			TotalAmount: job.TotalAmount,
			SharedMode:  job.SharedMode,
			Buyer: Person{
				Name: buyerName,
				Id:   job.Buyer,
			},
			SharedWith: sharedWith,
			Prompt:     job.Prompt,
		})

		if err != nil {
			slog.Error("error getting categorized spendings", "error", err)
			errCh <- err
			continue
		}

		job.Result = &result

		tx, err := p.db.Begin()
		if err != nil {
			slog.Error("error creating spending creating transaction", "error", err)
			errCh <- err
			continue
		}

		failed := false

		for _, spending := range job.Result.Spendings {
			var categoryId int

			categoryRow := tx.QueryRow(`SELECT id FROM categories WHERE name = ?`, spending.Category)

			err = categoryRow.Scan(&categoryId)

			if err != nil {
				tx.Rollback()
				failed = true
				slog.Error("error scanning category id", "error", err)
				errCh <- err
				break
			}

			result, err := tx.Exec(`INSERT INTO spendings (amount, description, category, made_by) 
			VALUES (?,?,?,?)`, spending.Amount, spending.Description, categoryId, job.Buyer)

			if err != nil {
				tx.Rollback()
				failed = true
				slog.Error("error inserting spending", "error", err)
				errCh <- err
				break
			}

			id, err := result.LastInsertId()
			if err != nil {
				tx.Rollback()
				failed = true
				slog.Error("error getting last inserted Id", "error", err)
				errCh <- err
				break
			}

			// Determine shared_with based on the individual spending's apportion_mode
			var sharedWithId *int64 = nil // Default to NULL (alone)
			sharedUserTakesAll := false

			switch spending.ApportionMode {
			case "shared":
				// If mode is 'shared', use the job's shared partner ID (if one exists)
				sharedWithId = job.SharedWithId // Stays nil if job.SharedWithId is nil
			case "other":
				// If mode is 'other', use the job's shared partner ID (must exist) and set flag
				if job.SharedWithId == nil {
					// This case should ideally be caught by validation in ProcessCategorizationJob,
					// but double-check here to prevent DB errors.
					tx.Rollback()
					failed = true
					slog.Error("logic error: apportion_mode is 'other' but job has no shared_with ID", "job_id", job.Id, "spending_desc", spending.Description)
					errCh <- fmt.Errorf("invalid state: apportion_mode 'other' without shared_with ID")
					break // Break inner loop
				}
				sharedWithId = job.SharedWithId
				sharedUserTakesAll = true
			case "alone":
				// sharedWithId remains nil
			default:
				// Should not happen due to validation in ProcessCategorizationJob
				tx.Rollback()
				failed = true
				slog.Error("logic error: invalid apportion_mode reached worker", "job_id", job.Id, "spending_desc", spending.Description, "mode", spending.ApportionMode)
				errCh <- fmt.Errorf("invalid state: invalid apportion_mode '%s'", spending.ApportionMode)
				break // Break inner loop
			}

			if failed { // Check if break was due to an error above
				break
			}

			_, err = tx.Exec(`INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all)
			VALUES (?,?,?,?)`, id, job.Buyer, sharedWithId, sharedUserTakesAll)

			if err != nil {
				tx.Rollback()
				failed = true
				slog.Error("error inserting user spending", "error", err)
				errCh <- err
				break
			}

			_, err = tx.Exec(`INSERT INTO ai_categorized_spendings (spending_id, job_id) 
			VALUES (?,?)`, id, job.Id)

			if err != nil {
				tx.Rollback()
				failed = true
				slog.Error("error inserting ai_categorized_spendings", "error", err)
				errCh <- err
				break
			}

		}

		if failed {
			continue
		}

		if err := tx.Commit(); err != nil {
			slog.Error("commiting transaction failed", "error", err)
			errCh <- err
			continue
		}

		_, err = p.db.Exec("UPDATE ai_categorization_jobs SET status = \"finished\", is_finished = true, is_ambiguity_flagged = ?, ambiguity_flag_reason = ?, status_updated_at = ? WHERE id = ?", job.Result.IsAmbiguityFlagged, job.Result.AmbiguityFlagReason, time.Now().UTC(), job.Id)

		if err != nil {
			slog.Error("failed when setting status to finished", "error", err)
			errCh <- err
			continue
		}
	}
}

func (p CategorizingPool) StartPool() {
	pollErrCh := make(chan error)
	errCh := make(chan error)

	go p.jobDbPoller(errCh)

	for w := 1; w <= p.numWorkers; w++ {
		go p.worker(p.unhandledJobs, errCh)
	}

	for {
		select {
		case err := <-pollErrCh:
			slog.Error("error when polling", "error", err)
		case err := <-errCh:
			slog.Error("error at worker", "error", err)
		}
	}
}
