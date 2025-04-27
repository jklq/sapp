package deposit

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
)

// HandleAddDeposit handles requests to add a new deposit record (protected).
func HandleAddDeposit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get authenticated user ID from context
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for adding deposit", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// 1. Decode the JSON payload
		var payload AddDepositPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode add deposit request body", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 2. Validate payload
		if payload.Amount <= 0 {
			slog.Warn("invalid add deposit payload: non-positive amount", "url", r.URL, "user_id", userID, "payload", payload)
			http.Error(w, "Bad Request: Amount must be positive", http.StatusBadRequest)
			return
		}
		if payload.Description == "" {
			slog.Warn("invalid add deposit payload: missing description", "url", r.URL, "user_id", userID, "payload", payload)
			http.Error(w, "Bad Request: Description is required", http.StatusBadRequest)
			return
		}
		// Validate date format (expecting YYYY-MM-DD)
		depositDate, err := time.Parse("2006-01-02", payload.DepositDate)
		if err != nil {
			slog.Warn("invalid add deposit payload: invalid date format", "url", r.URL, "user_id", userID, "date_string", payload.DepositDate, "err", err)
			http.Error(w, "Bad Request: Invalid date format (use YYYY-MM-DD)", http.StatusBadRequest)
			return
		}
		// Validate recurrence period if recurring
		if payload.IsRecurring && (payload.RecurrencePeriod == nil || *payload.RecurrencePeriod == "") {
			slog.Warn("invalid add deposit payload: missing recurrence period for recurring deposit", "url", r.URL, "user_id", userID, "payload", payload)
			http.Error(w, "Bad Request: Recurrence period is required for recurring deposits", http.StatusBadRequest)
			return
		}
		if !payload.IsRecurring {
			payload.RecurrencePeriod = nil // Ensure period is NULL if not recurring
		}

		// 3. Insert into database
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for adding deposit", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Rollback on error

		insertQuery := `
			INSERT INTO deposits (user_id, amount, description, deposit_date, is_recurring, recurrence_period)
			VALUES (?, ?, ?, ?, ?, ?)
		`
		result, err := tx.Exec(insertQuery, userID, payload.Amount, payload.Description, depositDate.Format("2006-01-02 15:04:05"), payload.IsRecurring, payload.RecurrencePeriod)
		if err != nil {
			slog.Error("failed to insert deposit", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		depositID, err := result.LastInsertId()
		if err != nil {
			slog.Error("failed to get last insert ID for deposit", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 4. Commit transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for adding deposit", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Deposit added successfully", "url", r.URL, "user_id", userID, "deposit_id", depositID)

		// 5. Respond with success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(AddDepositResponse{
			Message:   "Deposit added successfully",
			DepositID: depositID,
		})
	}
}

// HandleGetDeposits returns an http.HandlerFunc that fetches deposits for the logged-in user.
// Note: This might be merged into the main history endpoint later.
func HandleGetDeposits(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting deposits", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		query := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at
			FROM deposits
			WHERE user_id = ?
			ORDER BY deposit_date DESC, created_at DESC;
		`
		rows, err := db.Query(query, userID)
		if err != nil {
			slog.Error("failed to query deposits", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		deposits := []Deposit{}
		for rows.Next() {
			var d Deposit
			// Scan deposit_date directly into time.Time field
			if err := rows.Scan(
				&d.ID,
				&d.UserID,
				&d.Amount,
				&d.Description,
				&d.DepositDate, // Scan directly into time.Time
				&d.IsRecurring,
				&d.RecurrencePeriod,
				&d.CreatedAt,
			); err != nil {
				// Log the error, including the specific row data if possible (might require more complex scanning)
				slog.Error("failed to scan deposit row", "url", r.URL, "user_id", userID, "err", err)
				// If scanning fails, we might get a zero time. Consider how to handle this.
				// For now, let the loop continue, but the specific item might be incomplete.
				// Depending on requirements, you might want to return an error immediately.
				// Let's return an error to be safe.
				http.Error(w, "Internal server error during data retrieval", http.StatusInternalServerError)
				return
			}

			// No manual parsing needed anymore. The driver handles the conversion.
			// If the scan failed above, d.DepositDate might be zero, but the error would have been caught.

			deposits = append(deposits, d)
		}

		if err := rows.Err(); err != nil {
			slog.Error("error iterating deposit rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(deposits); err != nil {
			slog.Error("failed to encode deposits to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
	}
}
