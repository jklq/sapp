package export

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/types"
)

// HandleExportAllData generates a JSON export of all data for the user and their partner.
func HandleExportAllData(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for export", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		slog.Info("Starting data export process", "user_id", userID)

		// Use a single transaction for consistency, although it might be long-running.
		// Consider read-only transaction if supported and sufficient.
		tx, err := db.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true}) // Attempt read-only transaction
		if err != nil {
			slog.Error("failed to begin read-only transaction for export", "user_id", userID, "err", err)
			// Fallback to regular transaction if read-only is not supported/fails
			tx, err = db.Begin()
			if err != nil {
				slog.Error("failed to begin transaction for export", "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}
		defer tx.Rollback() // Ensure rollback happens, although it's read-only ideally

		// 1. Get User and Partner Info
		user, err := fetchUserExport(tx, userID)
		if err != nil {
			handleExportError(w, "fetching user details", userID, err)
			return
		}

		partnerID, partnerFound := auth.GetPartnerUserID(tx, userID)
		var partner types.UserExport
		if partnerFound {
			partner, err = fetchUserExport(tx, partnerID)
			if err != nil {
				handleExportError(w, "fetching partner details", userID, err)
				return
			}
		} else {
			slog.Warn("No partner found for user during export", "user_id", userID)
			// Partner will be an empty struct, which is fine for JSON marshalling
		}

		// 2. Get Categories (Global)
		categories, err := fetchCategoriesExport(tx)
		if err != nil {
			handleExportError(w, "fetching categories", userID, err)
			return
		}

		// 3. Get AI Jobs (for both user and partner)
		aiJobs, err := fetchAIJobsExport(tx, userID, partnerID)
		if err != nil {
			handleExportError(w, "fetching AI jobs", userID, err)
			return
		}

		// 4. Get Manual Spendings (for both user and partner)
		manualSpendings, err := fetchManualSpendingsExport(tx, userID, partnerID)
		if err != nil {
			handleExportError(w, "fetching manual spendings", userID, err)
			return
		}

		// 5. Get Deposits (for both user and partner)
		deposits, err := fetchDepositsExport(tx, userID, partnerID)
		if err != nil {
			handleExportError(w, "fetching deposits", userID, err)
			return
		}

		// 6. Get Transfers (between user and partner)
		transfers, err := fetchTransfersExport(tx, userID, partnerID)
		if err != nil {
			handleExportError(w, "fetching transfers", userID, err)
			return
		}

		// 7. Assemble Full Export Data
		exportData := types.FullExport{
			ExportedAt:      time.Now().UTC(),
			User:            user,
			Partner:         partner,
			Categories:      categories,
			AIJobs:          aiJobs,
			ManualSpendings: manualSpendings,
			Deposits:        deposits,
			Transfers:       transfers,
		}

		// 8. Marshal to JSON
		jsonData, err := json.MarshalIndent(exportData, "", "  ") // Use indent for readability
		if err != nil {
			handleExportError(w, "marshalling data to JSON", userID, err)
			return
		}

		// 9. Set Headers and Send Response
		filename := fmt.Sprintf("sapp_export_%s.json", time.Now().UTC().Format("20060102_150405"))
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(jsonData)
		if err != nil {
			// Log error, but headers might already be sent
			slog.Error("failed to write JSON export data to response", "user_id", userID, "err", err)
		}

		slog.Info("Data export completed successfully", "user_id", userID, "filename", filename)
	}
}

// --- Helper Functions for Fetching Data ---

func handleExportError(w http.ResponseWriter, step string, userID int64, err error) {
	slog.Error(fmt.Sprintf("Export failed during %s", step), "user_id", userID, "err", err)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, fmt.Sprintf("Data not found during %s", step), http.StatusNotFound)
	} else {
		http.Error(w, "Internal server error during export", http.StatusInternalServerError)
	}
}

func fetchUserExport(q auth.Querier, userID int64) (types.UserExport, error) {
	var user types.UserExport
	err := q.QueryRow("SELECT username, first_name FROM users WHERE id = ?", userID).Scan(&user.Username, &user.FirstName)
	if err != nil {
		return types.UserExport{}, fmt.Errorf("querying user %d: %w", userID, err)
	}
	return user, nil
}

func fetchCategoriesExport(tx *sql.Tx) ([]types.CategoryExport, error) {
	rows, err := tx.Query("SELECT name, ai_notes FROM categories ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("querying categories: %w", err)
	}
	defer rows.Close()

	categories := []types.CategoryExport{}
	for rows.Next() {
		var cat types.CategoryExport
		var aiNotes sql.NullString
		if err := rows.Scan(&cat.Name, &aiNotes); err != nil {
			return nil, fmt.Errorf("scanning category row: %w", err)
		}
		if aiNotes.Valid {
			cat.AINotes = &aiNotes.String
		}
		categories = append(categories, cat)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating category rows: %w", err)
	}
	return categories, nil
}

func fetchAIJobsExport(tx *sql.Tx, userID, partnerID int64) ([]types.AIJobExport, error) {
	jobs := []types.AIJobExport{}
	jobQuery := `
		SELECT
			j.id, j.prompt, j.total_amount, j.transaction_date, j.pre_settled,
			u.username AS buyer_username, j.is_ambiguity_flagged, j.ambiguity_flag_reason
		FROM ai_categorization_jobs j
		JOIN users u ON j.buyer = u.id
		WHERE j.buyer = ? OR j.buyer = ?
		ORDER BY j.transaction_date DESC, j.created_at DESC;
	`
	jobRows, err := tx.Query(jobQuery, userID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("querying AI jobs: %w", err)
	}
	defer jobRows.Close()

	spendingQuery := `
		SELECT
			c.name AS category_name, s.amount, s.description,
			us.shared_with, us.shared_user_takes_all
		FROM spendings s
		JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
		JOIN user_spendings us ON s.id = us.spending_id
		JOIN categories c ON s.category = c.id
		WHERE acs.job_id = ?
		ORDER BY s.id ASC;
	`
	spendingStmt, err := tx.Prepare(spendingQuery)
	if err != nil {
		return nil, fmt.Errorf("preparing spending query for AI jobs: %w", err)
	}
	defer spendingStmt.Close()

	for jobRows.Next() {
		var job types.AIJobExport
		var jobID int64 // Need the ID to query spendings
		var ambiguityReason sql.NullString

		if err := jobRows.Scan(
			&jobID, &job.Prompt, &job.TotalAmount, &job.TransactionDate, &job.PreSettled,
			&job.BuyerUsername, &job.IsAmbiguous, &ambiguityReason,
		); err != nil {
			return nil, fmt.Errorf("scanning AI job row: %w", err)
		}
		if ambiguityReason.Valid {
			job.AmbiguityReason = &ambiguityReason.String
		}

		spendingRows, err := spendingStmt.Query(jobID)
		if err != nil {
			return nil, fmt.Errorf("querying spendings for job %d: %w", jobID, err)
		}

		job.Spendings = []types.SpendingItemExport{}
		for spendingRows.Next() {
			var item types.SpendingItemExport
			var sharedWith sql.NullInt64
			var sharedUserTakesAll bool

			if err := spendingRows.Scan(
				&item.CategoryName, &item.Amount, &item.Description,
				&sharedWith, &sharedUserTakesAll,
			); err != nil {
				spendingRows.Close()
				return nil, fmt.Errorf("scanning spending item row for job %d: %w", jobID, err)
			}

			// Determine ApportionMode string
			if !sharedWith.Valid {
				item.ApportionMode = "Alone"
			} else if sharedUserTakesAll {
				item.ApportionMode = "PaidByPartner"
			} else {
				item.ApportionMode = "Shared"
			}
			job.Spendings = append(job.Spendings, item)
		}
		spendingRows.Close()
		if err := spendingRows.Err(); err != nil {
			return nil, fmt.Errorf("iterating spending item rows for job %d: %w", jobID, err)
		}
		jobs = append(jobs, job)
	}
	if err := jobRows.Err(); err != nil {
		return nil, fmt.Errorf("iterating AI job rows: %w", err)
	}
	return jobs, nil
}

func fetchManualSpendingsExport(tx *sql.Tx, userID, partnerID int64) ([]types.ManualSpendingExport, error) {
	spendings := []types.ManualSpendingExport{}
	query := `
		SELECT
			s.amount, s.description, c.name AS category_name, s.spending_date,
			u.username AS buyer_username, us.shared_with, us.shared_user_takes_all, us.settled_at
		FROM spendings s
		JOIN user_spendings us ON s.id = us.spending_id
		JOIN categories c ON s.category = c.id
		JOIN users u ON us.buyer = u.id
		LEFT JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
		WHERE
			acs.job_id IS NULL -- Only include spendings NOT linked to an AI job
			AND (us.buyer = ? OR us.buyer = ?) -- Include if bought by user or partner
		ORDER BY s.spending_date DESC, s.created_at DESC;
	`
	rows, err := tx.Query(query, userID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("querying manual spendings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sp types.ManualSpendingExport
		var sharedWith sql.NullInt64
		var sharedUserTakesAll bool
		var settledAt sql.NullTime

		if err := rows.Scan(
			&sp.Amount, &sp.Description, &sp.CategoryName, &sp.SpendingDate,
			&sp.BuyerUsername, &sharedWith, &sharedUserTakesAll, &settledAt,
		); err != nil {
			return nil, fmt.Errorf("scanning manual spending row: %w", err)
		}

		// Determine SharedStatus string
		if !sharedWith.Valid {
			sp.SharedStatus = "Alone"
		} else if sharedUserTakesAll {
			sp.SharedStatus = "PaidByPartner"
		} else {
			sp.SharedStatus = "Shared"
		}

		if settledAt.Valid {
			sp.SettledAt = &settledAt.Time
		}

		spendings = append(spendings, sp)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating manual spending rows: %w", err)
	}
	return spendings, nil
}

func fetchDepositsExport(tx *sql.Tx, userID, partnerID int64) ([]types.DepositExport, error) {
	deposits := []types.DepositExport{}
	query := `
		SELECT
			d.description, d.amount, d.deposit_date, d.is_recurring, d.recurrence_period, d.end_date,
			u.username AS owner_username
		FROM deposits d
		JOIN users u ON d.user_id = u.id
		WHERE d.user_id = ? OR d.user_id = ?
		ORDER BY d.deposit_date DESC, d.created_at DESC;
	`
	rows, err := tx.Query(query, userID, partnerID)
	if err != nil {
		return nil, fmt.Errorf("querying deposits: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var dep types.DepositExport
		var recurrencePeriod sql.NullString
		var endDate sql.NullTime

		if err := rows.Scan(
			&dep.Description, &dep.Amount, &dep.DepositDate, &dep.IsRecurring,
			&recurrencePeriod, &endDate, &dep.OwnerUsername,
		); err != nil {
			return nil, fmt.Errorf("scanning deposit row: %w", err)
		}

		if recurrencePeriod.Valid {
			dep.RecurrencePeriod = &recurrencePeriod.String
		}
		if endDate.Valid {
			dep.EndDate = &endDate.Time
		}
		deposits = append(deposits, dep)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating deposit rows: %w", err)
	}
	return deposits, nil
}

func fetchTransfersExport(tx *sql.Tx, userID, partnerID int64) ([]types.TransferExport, error) {
	transfers := []types.TransferExport{}
	query := `
		SELECT
			t.settlement_time, u.username AS settled_by_username
		FROM transfers t
		JOIN users u ON t.settled_by_user_id = u.id
		WHERE (t.settled_by_user_id = ? AND t.settled_with_user_id = ?)
		   OR (t.settled_by_user_id = ? AND t.settled_with_user_id = ?)
		ORDER BY t.settlement_time DESC;
	`
	rows, err := tx.Query(query, userID, partnerID, partnerID, userID)
	if err != nil {
		return nil, fmt.Errorf("querying transfers: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var tr types.TransferExport
		if err := rows.Scan(&tr.SettlementTime, &tr.SettledByUsername); err != nil {
			return nil, fmt.Errorf("scanning transfer row: %w", err)
		}
		transfers = append(transfers, tr)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating transfer rows: %w", err)
	}
	return transfers, nil
}
