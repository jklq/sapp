package category

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time" // Added time import
)

// SharedMode removed from Job struct
type Job struct {
	Id           int64 `json:"id"`
	TotalAmount  float64
	Status       string `json:"status"`
	IsFinished   bool   `json:"isFinished"`
	Prompt       string `json:"prompt"`
	Buyer           int64      `json:"buyer_id"`
	SharedWithId    *int64     `json:"other_id"`
	PreSettled      bool       `json:"pre_settled"` // Added: Pre-settled flag from the job table
	Result          *JobResult `json:"result"`      // Removed duplicate Result field
	TransactionDate *time.Time `json:"transaction_date"` // Renamed from SpendingDate to match DB schema
}

type CategorizingPoolStrategy interface {
	// AddJob adds a new categorization job with optional transaction date.
	AddJob(params CategorizationParams, transactionDate *time.Time) (int64, error)
	StartPool()
	GetStatus(int64) (Job, error)
}

type CategorizingPool struct {
	db            *sql.DB
	numWorkers    int
	unhandledJobs chan Job
	api           ModelAPI
}

// NewCategorizingPool now accepts a ModelAPI implementation.
func NewCategorizingPool(db *sql.DB, numWorkers int, api ModelAPI) CategorizingPool {
	return CategorizingPool{
		db:            db,
		numWorkers:    numWorkers,
		unhandledJobs: make(chan Job, 100),
		api:           api, // Store the provided API
	}
}

// AddJob adds a new categorization job to the queue.
// It inserts the job details (including optional transaction date) into the database
// and sends the job details to the channel for processing.
func (p *CategorizingPool) AddJob(params CategorizationParams, transactionDate *time.Time) (int64, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var otherPersonInt *int64 = nil

	if params.SharedWith != nil {
		otherPersonInt = &params.SharedWith.Id
	}

	// Store pre_settled flag and transaction_date in the job record
	// Pass transactionDate directly (can be nil). DB column is transaction_date.
	result, err := tx.Exec(`INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, pre_settled, transaction_date, status)
	VALUES (?, ?, ?, ?, ?, ?, ?)`, params.Buyer.Id, otherPersonInt, params.Prompt, params.TotalAmount, params.PreSettled, transactionDate, "pending")
	if err != nil {
		slog.Error("error inserting ai categorization job", "error", err, "pre_settled", params.PreSettled, "transaction_date", transactionDate)
		return 0, err
	}

	jobId, err := result.LastInsertId()
	if err != nil {
		slog.Error("error getting ai categorization job ID", "error", err)
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Send job details to the worker channel (non-blocking send might be better if channel can fill)
	// We pass the full job details needed by the worker.
	// Create the Job struct to send to the worker channel
	job := Job{
		Id: jobId,
		// Populate other fields from params as needed by the worker
		TotalAmount:  params.TotalAmount,
		Prompt:       params.Prompt,
		Buyer:           params.Buyer.Id,
		SharedWithId:    otherPersonInt,
		PreSettled:      params.PreSettled,
		TransactionDate: transactionDate, // Pass the parsed transaction date (or nil)
		Status:          "pending",
		// Result is initially nil
	}
	// Consider adding a timeout or select with a default case for non-blocking send
	p.unhandledJobs <- job
	slog.Debug("Job sent to worker channel", "job_id", jobId)

	return jobId, nil
}

// StartPool launches the worker goroutines.
func (p *CategorizingPool) StartPool() {
	for i := 1; i <= p.numWorkers; i++ {
		go p.worker(i)
	}
	slog.Info("Categorization pool workers started", "count", p.numWorkers)
}

func (p *CategorizingPool) GetStatus(id int64) (Job, error) {
	var jobStatus Job

	// Use p.db directly as receiver is now a pointer
	row := p.db.QueryRow("SELECT id, status, is_finished FROM ai_categorization_jobs WHERE id = ?", id)

	if err := row.Scan(&jobStatus.Id, &jobStatus.Status, &jobStatus.IsFinished); err != nil {
		return jobStatus, err
	}

	if !jobStatus.IsFinished {
		return jobStatus, nil
	}

	// TODO: Populate JobResult more completely if needed by the GetStatus consumer.
	// Currently, this function is not used by tested handlers.
	// Fetch transaction_date as well if needed here.
	// row := p.db.QueryRow("SELECT id, status, is_finished, transaction_date FROM ai_categorization_jobs WHERE id = ?", id)
	// var sqlTransactionDate sql.NullTime
	// ... scan &jobStatus.Id, &jobStatus.Status, &jobStatus.IsFinished, &sqlTransactionDate ...
	// if sqlTransactionDate.Valid { jobStatus.TransactionDate = &sqlTransactionDate.Time }


	if !jobStatus.IsFinished {
		return jobStatus, nil
	}

	// Example of fetching associated spendings if needed (adapt query as necessary)
	rows, err := p.db.Query(`SELECT s.id, s.amount, s.description -- Add other fields like category_name, spending_date if needed
	FROM spendings s
	INNER JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
	WHERE acs.job_id = ?`, jobStatus.Id)

	if err != nil {
		return jobStatus, err
	}

	for rows.Next() {
		fmt.Println("one spending!")
		t := Spendings{}
		err := rows.Scan(&t.Id, &t.Amount, &t.Description)
		if err != nil {
			return jobStatus, err
		}
		jobStatus.Result.Spendings = append(jobStatus.Result.Spendings, t)
	}

	return jobStatus, nil
}

// worker is the main loop for a categorization worker goroutine.
func (p *CategorizingPool) worker(id int) {
	slog.Info("Starting worker", "worker_id", id)
	for job := range p.unhandledJobs {
		slog.Info("Worker picked up job", "worker_id", id, "job_id", job.Id)
		p.updateJobStatus(job.Id, "processing", nil) // Mark as processing

		// --- Process Job ---
		// Reconstruct CategorizationParams needed by ProcessCategorizationJob
		// This assumes you have access to Buyer and SharedWith details if needed,
		// or modify ProcessCategorizationJob to accept IDs/less info if sufficient.
		// For now, let's assume ProcessCategorizationJob needs the full params.
		// We might need to query user/partner names here based on IDs in the job struct
		// if ProcessCategorizationJob requires them.
		// Simplified: Assuming ProcessCategorizationJob is adapted or params are reconstructed.
		// If ProcessCategorizationJob strictly needs the original params struct,
		// we should pass it through the channel instead of individual fields.
		// Let's assume for now ProcessCategorizationJob is flexible or adapted.
		// We need to reconstruct the CategorizationParams for ProcessCategorizationJob
		// This might require fetching user/partner names if the prompt generation needs them.
		// Simplified example (assuming ProcessCategorizationJob adapted or names not needed):
		paramsForProcessing := CategorizationParams{
			TotalAmount: job.TotalAmount,
			Buyer:       Person{Id: job.Buyer}, // Name might be missing here
			// SharedWith needs reconstruction if ID exists
			Prompt:     job.Prompt,
			PreSettled: job.PreSettled,
			// tries is handled internally
		}
		if job.SharedWithId != nil {
			// Potentially fetch partner name here if needed by getPrompt
			paramsForProcessing.SharedWith = &Person{Id: *job.SharedWithId} // Name might be missing
		}

		// Pass the stored ModelAPI to ProcessCategorizationJob
		jobResult, err := ProcessCategorizationJob(p.db, p.api, paramsForProcessing)
		if err != nil {
			slog.Error("Worker failed to process job", "worker_id", id, "job_id", job.Id, "err", err)
			p.updateJobStatus(job.Id, "failed", err) // Update status with error
			continue                                 // Move to the next job
		}

		// --- Insert Spendings into DB ---
		tx, err := p.db.Begin()
		if err != nil {
			slog.Error("worker failed to begin transaction", "worker_id", id, "job_id", job.Id, "err", err)
			p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error: %w", err))
			continue
		}
		// Use defer with a named return to handle rollback correctly on errors inside the loop
		func() {
			defer func() {
				if r := recover(); r != nil {
					tx.Rollback()
					slog.Error("Worker recovered from panic during transaction", "worker_id", id, "job_id", job.Id, "panic", r)
					p.updateJobStatus(job.Id, "failed", fmt.Errorf("panic: %v", r))
				} else if err != nil {
					tx.Rollback() // Rollback if any error occurred before commit
					slog.Error("Worker rolling back transaction due to error", "worker_id", id, "job_id", job.Id, "err", err)
					// Status already updated where error occurred
				}
			}()


			// Determine settled_at based on job parameters
			var settledAt sql.NullTime
			if job.PreSettled { // Use job.PreSettled from the Job struct
				settledAt = sql.NullTime{Time: time.Now().UTC(), Valid: true}
			}


			// Determine the transaction_date to use for all items in this job.
			var transactionDateToUse time.Time
			if job.TransactionDate != nil {
				transactionDateToUse = *job.TransactionDate
				slog.Debug("Worker using provided transaction date for job", "worker_id", id, "job_id", job.Id, "date", transactionDateToUse)
			} else {
				// Fetch job creation time as fallback if specific date wasn't provided
				// Note: The DB column is transaction_date, but we might still want creation_date as fallback?
				// Let's try fetching transaction_date first, then created_at.
				var dbTransactionDate sql.NullTime
				err = tx.QueryRow("SELECT transaction_date FROM ai_categorization_jobs WHERE id = ?", job.Id).Scan(&dbTransactionDate)
				if err == nil && dbTransactionDate.Valid {
					transactionDateToUse = dbTransactionDate.Time
					slog.Debug("Worker using job's transaction_date from DB", "worker_id", id, "job_id", job.Id, "date", transactionDateToUse)
				} else {
					// If transaction_date is NULL or query failed, fallback to created_at
					slog.Warn("Worker could not get transaction_date from DB, falling back to created_at", "worker_id", id, "job_id", job.Id, "err", err)
					err = tx.QueryRow("SELECT created_at FROM ai_categorization_jobs WHERE id = ?", job.Id).Scan(&transactionDateToUse)
					if err != nil {
						slog.Error("Worker failed to query job creation time as fallback date, using current time", "worker_id", id, "job_id", job.Id, "err", err)
						transactionDateToUse = time.Now().UTC() // Final fallback to now
						err = nil                               // Clear the error so we don't rollback unnecessarily
					} else {
						slog.Debug("Worker using job creation date as transaction date", "worker_id", id, "job_id", job.Id, "date", transactionDateToUse)
					}
				}
			}


			// Pre-fetch category IDs needed for this job
			var categoryIDs map[string]int64
			categoryIDs, err = p.fetchCategoryIDs(tx, jobResult.Spendings)
			if err != nil {
				slog.Error("worker failed to fetch category IDs", "worker_id", id, "job_id", job.Id, "err", err)
				p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error fetching categories: %w", err))
				return // Error will trigger rollback
			}


			for _, spending := range jobResult.Spendings {
				var categoryID int64
				var ok bool
				categoryID, ok = categoryIDs[spending.Category]
				if !ok || categoryID == 0 { // Check if category was found and has a valid ID
					err = fmt.Errorf("category '%s' not found or invalid", spending.Category)
					slog.Error("worker category validation failed", "worker_id", id, "job_id", job.Id, "category", spending.Category, "err", err)
					p.updateJobStatus(job.Id, "failed", err)
					return // Error will trigger rollback
				}


				// 1. Insert into spendings
				spendingDesc := spending.Description
				if spendingDesc == "" {
					spendingDesc = "AI Categorized" // Default description
				}
				var res sql.Result
				// Use transactionDateToUse for the spending_date column in spendings table
				res, err = tx.Exec(`INSERT INTO spendings (amount, description, category, made_by, spending_date)
				VALUES (?, ?, ?, ?, ?)`,
					spending.Amount, spendingDesc, categoryID, job.Buyer, transactionDateToUse) // Use job.Buyer and determined transactionDateToUse
				if err != nil {
					slog.Error("worker failed to insert spending", "worker_id", id, "job_id", job.Id, "err", err)
					p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error inserting spending: %w", err))
					return // Error will trigger rollback
				}


				var spendingID int64
				spendingID, err = res.LastInsertId()
				if err != nil {
					slog.Error("worker failed to get last insert ID for spending", "worker_id", id, "job_id", job.Id, "err", err)
					p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error getting spending ID: %w", err))
					return // Error will trigger rollback
				}


				// 2. Insert into ai_categorized_spendings
				_, err = tx.Exec(`INSERT INTO ai_categorized_spendings (job_id, spending_id) VALUES (?, ?)`, job.Id, spendingID)
				if err != nil {
					slog.Error("worker failed to insert ai_categorized_spending", "worker_id", id, "job_id", job.Id, "spending_id", spendingID, "err", err)
					p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error inserting categorized spending link: %w", err))
					return // Error will trigger rollback
				}


				// 3. Insert into user_spendings
				var sharedWithID sql.NullInt64
				sharedUserTakesAll := false // Default

				// Use job.SharedWithId directly instead of job.Params
				if job.SharedWithId != nil {
					sharedWithID = sql.NullInt64{Int64: *job.SharedWithId, Valid: true}
					// Determine sharedUserTakesAll based on the validated apportion_mode
					// The AI should return "other" if the partner pays all.
					if spending.ApportionMode == "other" {
						sharedUserTakesAll = true
					}
					// Note: If apportion_mode is "shared", sharedUserTakesAll remains false.
					// If apportion_mode is "alone", this block shouldn't be reached if logic is correct,
					// but sharedUserTakesAll would remain false anyway.
				} else {
					// If there's no shared partner, it must be 'alone'
					sharedWithID = sql.NullInt64{Valid: false}
					sharedUserTakesAll = false
					// Add a check here? If spending.ApportionMode is not 'alone', it's an error state.
					if spending.ApportionMode != "alone" {
						err = fmt.Errorf("logic error: spending mode '%s' found but no partner associated with job %d", spending.ApportionMode, job.Id)
						slog.Error("worker invalid spending mode for non-shared job", "worker_id", id, "job_id", job.Id, "mode", spending.ApportionMode, "err", err)
						p.updateJobStatus(job.Id, "failed", err)
						return // Error will trigger rollback
					}
				}


				_, err = tx.Exec(`INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all, settled_at)
				VALUES (?, ?, ?, ?, ?)`,
					spendingID, job.Buyer, sharedWithID, sharedUserTakesAll, settledAt) // Use job.Buyer
				if err != nil {
					slog.Error("worker failed to insert user_spending", "worker_id", id, "job_id", job.Id, "spending_id", spendingID, "err", err)
					p.updateJobStatus(job.Id, "failed", fmt.Errorf("db error inserting user_spending: %w", err))
					return // Error will trigger rollback
				}
			} // End loop through spendings


			// --- Finalize Job ---
			// Commit transaction if no errors occurred within the loop
			err = tx.Commit()
			if err != nil {
				slog.Error("worker failed to commit transaction", "worker_id", id, "job_id", job.Id, "err", err)
				p.updateJobStatus(job.Id, "failed", fmt.Errorf("db commit error: %w", err))
				// Rollback already deferred
				return
			}


			// Update job status to completed and store ambiguity flag/reason AFTER successful commit
			finalStatus := "completed"
			var ambiguityReason sql.NullString
			if jobResult.IsAmbiguityFlagged {
				ambiguityReason = sql.NullString{String: jobResult.AmbiguityFlagReason, Valid: true}
			}
			p.updateJobStatusAndAmbiguity(job.Id, finalStatus, nil, jobResult.IsAmbiguityFlagged, ambiguityReason) // Pass nil error on success


			slog.Info("Worker finished processing job successfully", "worker_id", id, "job_id", job.Id)
		}() // End of func literal for defer tx.Rollback()
	}
	slog.Info("Worker shutting down", "worker_id", id)
}


// fetchCategoryIDs pre-fetches category IDs for the given spending items within a transaction.
// Uses standard library database/sql.
func (p *CategorizingPool) fetchCategoryIDs(tx *sql.Tx, spendings []Spendings) (map[string]int64, error) {
	categoryIDs := make(map[string]int64)
	catNamesMap := make(map[string]struct{}) // Use map for unique names


	for _, sp := range spendings {
		if sp.Category != "" {
			catNamesMap[sp.Category] = struct{}{}
		}
	}


	if len(catNamesMap) == 0 {
		return categoryIDs, nil // No categories to fetch
	}


	// Convert map keys to slice for query
	catNames := make([]string, 0, len(catNamesMap))
	for name := range catNamesMap {
		catNames = append(catNames, name)
	}


	// Build IN clause placeholders
	placeholders := strings.Repeat("?,", len(catNames)-1) + "?"
	query := fmt.Sprintf("SELECT id, name FROM categories WHERE name IN (%s);", placeholders)


	// Convert slice of strings to slice of interface{} for Query args
	args := make([]interface{}, len(catNames))
	for i, v := range catNames {
		args[i] = v
	}


	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute category query: %w", err)
	}
	defer rows.Close()


	for rows.Next() {
		var catId int64
		var catName string
		if err := rows.Scan(&catId, &catName); err != nil {
			return nil, fmt.Errorf("failed to scan category row: %w", err)
		}
		categoryIDs[catName] = catId
	}


	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating category rows: %w", err)
	}


	// Verify all requested categories were found
	for name := range catNamesMap {
		if _, found := categoryIDs[name]; !found {
			slog.Warn("Category name provided by AI not found in database", "category_name", name)
			// Mark as not found / invalid ID. The loop inserting spendings will check for ID 0.
			categoryIDs[name] = 0
		}
	}


	return categoryIDs, nil
}


// updateJobStatus updates the status and error message of a job in the database.
func (p *CategorizingPool) updateJobStatus(jobID int64, status string, jobErr error) {
	var errMsg sql.NullString
	isFinished := false
	if status == "completed" || status == "failed" {
		isFinished = true
		if jobErr != nil {
			errMsg = sql.NullString{String: jobErr.Error(), Valid: true}
		}
	}


	_, err := p.db.Exec(`UPDATE ai_categorization_jobs SET status = ?, error_message = ?, is_finished = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, errMsg, isFinished, jobID)
	if err != nil {
		slog.Error("Failed to update job status in database", "job_id", jobID, "new_status", status, "update_err", err)
	} else {
		slog.Debug("Updated job status in database", "job_id", jobID, "new_status", status, "is_finished", isFinished)
	}
}


// updateJobStatusAndAmbiguity updates status, error, finished flag, and ambiguity details.
func (p *CategorizingPool) updateJobStatusAndAmbiguity(jobID int64, status string, jobErr error, isAmbiguous bool, ambiguityReason sql.NullString) {
	var errMsg sql.NullString
	isFinished := false
	if status == "completed" || status == "failed" {
		isFinished = true
		if jobErr != nil {
			errMsg = sql.NullString{String: jobErr.Error(), Valid: true}
		}
	}


	_, err := p.db.Exec(`UPDATE ai_categorization_jobs
		SET status = ?, error_message = ?, is_finished = ?, is_ambiguity_flagged = ?, ambiguity_flag_reason = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`,
		status, errMsg, isFinished, isAmbiguous, ambiguityReason, jobID)
	if err != nil {
		slog.Error("Failed to update job status and ambiguity in database", "job_id", jobID, "new_status", status, "update_err", err)
	} else {
		slog.Debug("Updated job status and ambiguity in database", "job_id", jobID, "new_status", status, "is_finished", isFinished, "is_ambiguous", isAmbiguous)
	}
}
