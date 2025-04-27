package deposit

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings" // Import strings package
	"time"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
	"git.sr.ht/~relay/sapp-backend/types" // Import shared types
)

// --- Helper Functions ---

// parseOptionalDate parses a date string ("YYYY-MM-DD") into a *time.Time pointer.
// Returns nil if the input string is empty or nil. Returns error on invalid format.
func parseOptionalDate(dateStr *string) (*time.Time, error) {
	if dateStr == nil || *dateStr == "" {
		return nil, nil // No date provided or explicitly cleared
	}
	parsedDate, err := time.Parse("2006-01-02", *dateStr)
	if err != nil {
		return nil, fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
	}
	// Return pointer to the parsed date
	// Ensure it's UTC if timezone matters, though date-only shouldn't be affected much
	parsedDate = parsedDate.UTC()
	return &parsedDate, nil
}

// --- Handlers ---

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
		var payload types.AddDepositPayload // Use types.AddDepositPayload
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
			slog.Warn("invalid add deposit payload: invalid date format", "url", r.URL, "user_id", userID, "date_string", payload.DepositDate, "err", err)
			http.Error(w, fmt.Sprintf("Bad Request: %s", err.Error()), http.StatusBadRequest) // Include specific error
			return
		}
		// Validate recurrence period if recurring
		if payload.IsRecurring {
			if payload.RecurrencePeriod == nil || *payload.RecurrencePeriod == "" {
				slog.Warn("invalid add deposit payload: missing recurrence period for recurring deposit", "url", r.URL, "user_id", userID, "payload", payload)
				http.Error(w, "Bad Request: Recurrence period is required for recurring deposits", http.StatusBadRequest)
				return
			}
			// Basic validation for known periods
			validPeriods := map[string]bool{"weekly": true, "monthly": true, "yearly": true} // Add more if needed
			if !validPeriods[*payload.RecurrencePeriod] {
				slog.Warn("invalid add deposit payload: unsupported recurrence period", "url", r.URL, "user_id", userID, "period", *payload.RecurrencePeriod)
				http.Error(w, "Bad Request: Unsupported recurrence period", http.StatusBadRequest)
				return
			}
		} else {
			payload.RecurrencePeriod = nil // Ensure period is NULL if not recurring
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

		// Note: end_date is not set during initial creation, it's NULL by default
		insertQuery := `
			INSERT INTO deposits (user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		// Use UTC time for consistency
		now := time.Now().UTC()
		// Format depositDate for storage (just date part is fine, but full timestamp is safer)
		depositDateStr := depositDate.Format("2006-01-02 15:04:05")

		result, err := tx.Exec(insertQuery, userID, payload.Amount, payload.Description, depositDateStr, payload.IsRecurring, payload.RecurrencePeriod, now)
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
		json.NewEncoder(w).Encode(types.AddDepositResponse{ // Use types.AddDepositResponse
			Message:   "Deposit added successfully",
			DepositID: depositID,
		})
	}
}

// HandleGetDepositByID fetches a single deposit template by its ID.
func HandleGetDepositByID(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting deposit by ID", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		depositIDStr := r.PathValue("deposit_id")
		depositID, err := strconv.ParseInt(depositIDStr, 10, 64)
		if err != nil {
			slog.Warn("invalid deposit ID format for get by ID", "url", r.URL, "user_id", userID, "deposit_id_str", depositIDStr, "err", err)
			http.Error(w, "Invalid deposit ID", http.StatusBadRequest)
			return
		}

		query := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at
			FROM deposits
			WHERE id = ? AND user_id = ?;
		`
		row := db.QueryRow(query, depositID, userID)

		var d types.Deposit // Use types.Deposit
		// Need to scan into nullable fields correctly
		var endDate sql.NullTime
		var recurrencePeriod sql.NullString

		if err := row.Scan(
			&d.ID,
			&d.UserID,
			&d.Amount,
			&d.Description,
			&d.DepositDate, // Scan directly into time.Time
			&d.IsRecurring,
			&recurrencePeriod, // Scan into sql.NullString
			&endDate,          // Scan into sql.NullTime
			&d.CreatedAt,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("deposit not found or access denied", "url", r.URL, "user_id", userID, "deposit_id", depositID)
				http.Error(w, "Deposit not found", http.StatusNotFound)
			} else {
				slog.Error("failed to scan deposit row for get by ID", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// Convert nullable fields to pointers in the struct
		if recurrencePeriod.Valid {
			d.RecurrencePeriod = &recurrencePeriod.String
		}
		if endDate.Valid {
			d.EndDate = &endDate.Time
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(d); err != nil {
			slog.Error("failed to encode deposit to JSON", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
		}
	}
}

// HandleUpdateDeposit handles requests to update an existing deposit record (protected).
func HandleUpdateDeposit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for updating deposit", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		depositIDStr := r.PathValue("deposit_id")
		depositID, err := strconv.ParseInt(depositIDStr, 10, 64)
		if err != nil {
			slog.Warn("invalid deposit ID format for update", "url", r.URL, "user_id", userID, "deposit_id_str", depositIDStr, "err", err)
			http.Error(w, "Invalid deposit ID", http.StatusBadRequest)
			return
		}

		// 1. Decode Payload
		var payload types.UpdateDepositPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode update deposit request body", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 2. Begin Transaction
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for update deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 3. Verify Ownership & Get Current State
		var current types.Deposit
		var currentEndDate sql.NullTime
		var currentRecurrencePeriod sql.NullString
		query := `SELECT user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at FROM deposits WHERE id = ?`
		err = tx.QueryRow(query, depositID).Scan(
			&current.UserID, &current.Amount, &current.Description, &current.DepositDate,
			&current.IsRecurring, &currentRecurrencePeriod, &currentEndDate, &current.CreatedAt,
		)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("update deposit attempt on non-existent deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID)
				http.Error(w, "Deposit not found", http.StatusNotFound)
			} else {
				slog.Error("failed to query current deposit state for update", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if current.UserID != userID {
			slog.Warn("unauthorized attempt to update deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "actual_owner_id", current.UserID)
			http.Error(w, "Forbidden: You can only update your own deposits", http.StatusForbidden)
			return
		}
		// Populate pointers from nullable fields for easier access
		if currentRecurrencePeriod.Valid {
			current.RecurrencePeriod = &currentRecurrencePeriod.String
		}
		if currentEndDate.Valid {
			current.EndDate = &currentEndDate.Time
		}

		// 4. Prepare Update Fields based on Payload (only update provided fields)
		updateFields := make(map[string]interface{})
		var newDepositDate *time.Time // Store parsed date if provided

		if payload.Amount != nil {
			if *payload.Amount <= 0 {
				http.Error(w, "Bad Request: Amount must be positive", http.StatusBadRequest)
				return
			}
			updateFields["amount"] = *payload.Amount
			current.Amount = *payload.Amount // Update current state for response
		}
		if payload.Description != nil {
			if *payload.Description == "" {
				http.Error(w, "Bad Request: Description cannot be empty", http.StatusBadRequest)
				return
			}
			updateFields["description"] = *payload.Description
			current.Description = *payload.Description // Update current state
		}
		if payload.DepositDate != nil {
			parsedDate, err := parseOptionalDate(payload.DepositDate) // Use YYYY-MM-DD
			if err != nil {
				http.Error(w, fmt.Sprintf("Bad Request: %s", err.Error()), http.StatusBadRequest)
				return
			}
			if parsedDate == nil { // Should not happen if string is not nil, but check defensively
				http.Error(w, "Bad Request: Invalid deposit date provided", http.StatusBadRequest)
				return
			}
			newDepositDate = parsedDate
			updateFields["deposit_date"] = newDepositDate.Format("2006-01-02 15:04:05")
			current.DepositDate = *newDepositDate // Update current state
		}

		// Handle recurrence logic carefully
		newIsRecurring := current.IsRecurring // Start with current value
		if payload.IsRecurring != nil {
			newIsRecurring = *payload.IsRecurring
			updateFields["is_recurring"] = newIsRecurring
			current.IsRecurring = newIsRecurring // Update current state
		}

		newRecurrencePeriod := current.RecurrencePeriod // Start with current value
		if payload.RecurrencePeriod != nil {
			if *payload.RecurrencePeriod == "" { // Allow clearing the period
				newRecurrencePeriod = nil
				updateFields["recurrence_period"] = nil
			} else {
				// Validate period if setting it
				validPeriods := map[string]bool{"weekly": true, "monthly": true, "yearly": true}
				if !validPeriods[*payload.RecurrencePeriod] {
					http.Error(w, "Bad Request: Unsupported recurrence period", http.StatusBadRequest)
					return
				}
				newRecurrencePeriod = payload.RecurrencePeriod
				updateFields["recurrence_period"] = *newRecurrencePeriod
			}
			current.RecurrencePeriod = newRecurrencePeriod // Update current state
		}

		// If now recurring, period must be set. If not recurring, period must be null.
		if newIsRecurring && (newRecurrencePeriod == nil || *newRecurrencePeriod == "") {
			http.Error(w, "Bad Request: Recurrence period is required for recurring deposits", http.StatusBadRequest)
			return
		}
		if !newIsRecurring && newRecurrencePeriod != nil {
			// If user sets is_recurring=false but doesn't clear period, clear it automatically
			slog.Debug("Clearing recurrence period because is_recurring is false", "deposit_id", depositID)
			updateFields["recurrence_period"] = nil
			current.RecurrencePeriod = nil // Update current state
		}

		// Handle end_date
		newEndDate := current.EndDate // Start with current value
		if payload.EndDate != nil {
			parsedEndDate, err := parseOptionalDate(payload.EndDate) // Use YYYY-MM-DD
			if err != nil {
				http.Error(w, fmt.Sprintf("Bad Request: Invalid end date format: %s", err.Error()), http.StatusBadRequest)
				return
			}
			newEndDate = parsedEndDate // Can be nil if payload.EndDate was "" or null
			updateFields["end_date"] = newEndDate // Store *time.Time directly, driver handles nil
			current.EndDate = newEndDate          // Update current state
		}

		// If not recurring, end_date should ideally be null (or maybe start date?)
		// Let's enforce NULL for simplicity.
		if !newIsRecurring && newEndDate != nil {
			slog.Debug("Clearing end date because is_recurring is false", "deposit_id", depositID)
			updateFields["end_date"] = nil
			current.EndDate = nil // Update current state
		}

		// 5. Execute Update if any fields changed
		if len(updateFields) == 0 {
			slog.Info("No fields to update for deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID)
			// Still return success with current data
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(types.UpdateDepositResponse{
				Message: "No changes detected",
				Deposit: current,
			})
			return
		}

		setClauses := []string{}
		args := []interface{}{}
		argIdx := 1
		for key, val := range updateFields {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", key, argIdx))
			args = append(args, val)
			argIdx++
		}
		args = append(args, depositID) // Add depositID for WHERE clause

		updateQuery := fmt.Sprintf("UPDATE deposits SET %s WHERE id = $%d",
			strings.Join(setClauses, ", "), argIdx) // This was the unused PostgreSQL query string

		// Rebuild query and args for SQLite format (?)
		setClausesSQLite := []string{}
		argsSQLite := []interface{}{}
		for key, val := range updateFields {
			setClausesSQLite = append(setClausesSQLite, fmt.Sprintf("%s = ?", key))
			argsSQLite = append(argsSQLite, val)
		}
		argsSQLite = append(argsSQLite, depositID) // Add depositID for WHERE clause
		updateQuerySQLite := fmt.Sprintf("UPDATE deposits SET %s WHERE id = ?",
			strings.Join(setClausesSQLite, ", "))

		_, err = tx.Exec(updateQuerySQLite, argsSQLite...)
		if err != nil {
			slog.Error("failed to execute deposit update", "url", r.URL, "user_id", userID, "deposit_id", depositID, "query", updateQuerySQLite, "args", argsSQLite, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 6. Commit Transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for update deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Deposit updated successfully", "url", r.URL, "user_id", userID, "deposit_id", depositID)

		// 7. Respond with success and updated deposit data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(types.UpdateDepositResponse{
			Message: "Deposit updated successfully",
			Deposit: current, // Return the updated state
		})
	}
}

// HandleDeleteDeposit handles requests to delete an existing deposit record (template).
func HandleDeleteDeposit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for deleting deposit", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		depositIDStr := r.PathValue("deposit_id")
		depositID, err := strconv.ParseInt(depositIDStr, 10, 64)
		if err != nil {
			slog.Warn("invalid deposit ID format for delete", "url", r.URL, "user_id", userID, "deposit_id_str", depositIDStr, "err", err)
			http.Error(w, "Invalid deposit ID", http.StatusBadRequest)
			return
		}

		// 1. Begin Transaction
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for delete deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback()

		// 2. Verify Ownership
		var ownerID int64
		err = tx.QueryRow("SELECT user_id FROM deposits WHERE id = ?", depositID).Scan(&ownerID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("delete deposit attempt on non-existent deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID)
				http.Error(w, "Deposit not found", http.StatusNotFound)
			} else {
				slog.Error("failed to query deposit owner for delete", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if ownerID != userID {
			slog.Warn("unauthorized attempt to delete deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "actual_owner_id", ownerID)
			http.Error(w, "Forbidden: You can only delete your own deposits", http.StatusForbidden)
			return
		}

		// 3. Execute Delete (Hard Delete)
		result, err := tx.Exec("DELETE FROM deposits WHERE id = ?", depositID)
		if err != nil {
			slog.Error("failed to execute deposit delete", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Check if any row was actually deleted
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			// Log error but proceed, the delete likely worked if no error previously
			slog.Error("failed to get rows affected after delete deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
		}
		if rowsAffected == 0 {
			// This shouldn't happen if the ownership check passed, but handle defensively
			slog.Warn("delete deposit executed but no rows were affected", "url", r.URL, "user_id", userID, "deposit_id", depositID)
			http.Error(w, "Deposit not found", http.StatusNotFound) // Treat as not found if nothing deleted
			return
		}


		// 4. Commit Transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for delete deposit", "url", r.URL, "user_id", userID, "deposit_id", depositID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Deposit deleted successfully", "url", r.URL, "user_id", userID, "deposit_id", depositID)

		// 5. Respond with success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200 OK with message is fine, or 204 No Content
		json.NewEncoder(w).Encode(types.DeleteDepositResponse{
			Message: "Deposit deleted successfully",
		})
	}
}


// HandleGetDeposits returns an http.HandlerFunc that fetches deposit templates for the logged-in user.
func HandleGetDeposits(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting deposit templates", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Fetch deposit templates, including the new end_date
		query := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at
			FROM deposits
			WHERE user_id = ? -- Removed is_active filter, assuming hard delete
			ORDER BY deposit_date DESC, created_at DESC;
		`
		rows, err := db.Query(query, userID)
		if err != nil {
			slog.Error("failed to query deposits", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		deposits := []types.Deposit{} // Use types.Deposit
		for rows.Next() {
			var d types.Deposit // Use types.Deposit
			// Need to scan into nullable fields correctly
			var endDate sql.NullTime
			var recurrencePeriod sql.NullString

			if err := rows.Scan(
				&d.ID,
				&d.UserID,
				&d.Amount,
				&d.Description,
				&d.DepositDate, // Scan directly into time.Time
				&d.IsRecurring,
				&recurrencePeriod, // Scan into sql.NullString
				&endDate,          // Scan into sql.NullTime
				&d.CreatedAt,
			); err != nil {
				slog.Error("failed to scan deposit template row", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error during data retrieval", http.StatusInternalServerError)
				return
			}

			// Convert nullable fields to pointers in the struct
			if recurrencePeriod.Valid {
				d.RecurrencePeriod = &recurrencePeriod.String
			}
			if endDate.Valid {
				d.EndDate = &endDate.Time
			}

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
