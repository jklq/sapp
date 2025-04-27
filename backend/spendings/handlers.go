package spendings

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"errors"
	"fmt"
	"strconv"
	"strings"

	"git.sr.ht/~relay/sapp-backend/auth"    // Import auth package
	"git.sr.ht/~relay/sapp-backend/history" // Import the new history service package
	"git.sr.ht/~relay/sapp-backend/types"   // Import shared types
)

// EditableSharingStatus moved to types package
// UpdateSpendingPayload moved to types package
// SpendingItem moved to types package
// DepositItem moved to types package
// TransactionGroup moved to types package

// FrontendHistoryListItem defines the structure for a single item in the history API response.
// It flattens the data from TransactionGroup and DepositItem.
type FrontendHistoryListItem struct {
	// Common fields
	Type string    `json:"type"` // "spending_group" or "deposit"
	Date time.Time `json:"date"` // Primary sorting key (job creation time or deposit occurrence date)

	// Fields from TransactionGroup (omitempty if not applicable)
	JobID               *int64             `json:"job_id,omitempty"`
	Prompt              *string            `json:"prompt,omitempty"`
	TotalAmount         *float64           `json:"total_amount,omitempty"` // Use pointer for optional field
	BuyerName           *string            `json:"buyer_name,omitempty"`
	IsAmbiguityFlagged  *bool              `json:"is_ambiguity_flagged,omitempty"`
	AmbiguityFlagReason *string            `json:"ambiguity_flag_reason,omitempty"`
	Spendings           []types.SpendingItem `json:"spendings,omitempty"` // Use types.SpendingItem

	// Fields from DepositItem (omitempty if not applicable)
	ID               *int64   `json:"id,omitempty"` // Deposit ID (original template ID)
	Amount           *float64 `json:"amount,omitempty"` // Use pointer for optional field
	Description      *string  `json:"description,omitempty"`
	IsRecurring      *bool    `json:"is_recurring,omitempty"`
	RecurrencePeriod *string  `json:"recurrence_period,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"` // Deposit template creation time
}

// HistoryResponse defines the structure for the combined history endpoint response.
type HistoryResponse struct {
	History []FrontendHistoryListItem `json:"history"`
}

// HandleGetHistory returns an http.HandlerFunc that fetches combined and sorted history items
// using the history service.
func HandleGetHistory(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting history", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Use the new history service to generate the combined history
		// Generate history up to the current time
		combinedHistory, err := history.GenerateHistory(db, userID, time.Now().UTC())
		if err != nil {
			slog.Error("failed to generate history using service", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Prepare the response structure
		historyResponse := HistoryResponse{
			History: make([]FrontendHistoryListItem, 0, len(combinedHistory)), // Initialize slice
		}

		// Populate the response by converting internal HistoryListItem to FrontendHistoryListItem
		for _, internalItem := range combinedHistory {
			frontendItem := FrontendHistoryListItem{
				Type: internalItem.Type,
				Date: internalItem.Date,
			}

			switch typedItem := internalItem.RawItem.(type) {
			case types.TransactionGroup:
				// Populate fields specific to TransactionGroup
				frontendItem.JobID = &typedItem.JobID // Use pointers for optional fields
				frontendItem.Prompt = &typedItem.Prompt
				frontendItem.TotalAmount = &typedItem.TotalAmount
				frontendItem.BuyerName = &typedItem.BuyerName
				frontendItem.IsAmbiguityFlagged = &typedItem.IsAmbiguityFlagged
				frontendItem.AmbiguityFlagReason = typedItem.AmbiguityFlagReason // Already a pointer
				frontendItem.Spendings = typedItem.Spendings
			case types.DepositItem:
				// Populate fields specific to DepositItem
				frontendItem.ID = &typedItem.ID // Use pointers for optional fields
				frontendItem.Amount = &typedItem.Amount
				frontendItem.Description = &typedItem.Description
				frontendItem.IsRecurring = &typedItem.IsRecurring
				frontendItem.RecurrencePeriod = typedItem.RecurrencePeriod // Already a pointer
				frontendItem.CreatedAt = &typedItem.CreatedAt
			default:
				slog.Warn("Unknown item type encountered in history list during conversion", "type", internalItem.Type)
				continue // Skip unknown types
			}
			historyResponse.History = append(historyResponse.History, frontendItem)
		}


		/*
		// --- Old logic replaced by history.GenerateHistory ---

		// 1. Fetch AI Categorization Jobs (Spending Groups) initiated by the user
		jobQuery := `
			SELECT
				j.id, j.prompt, j.total_amount, j.created_at, j.is_ambiguity_flagged, j.ambiguity_flag_reason, u.first_name AS buyer_name
		// ... (rest of the old fetching logic for jobs and spendings) ...

		// 3. Fetch Deposits for the user
		depositQuery := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at
			FROM deposits
			WHERE user_id = ?
			ORDER BY deposit_date DESC, created_at DESC;
		`
		depositRows, err := db.Query(depositQuery, userID)
		if err != nil {
			slog.Error("failed to query deposits for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer depositRows.Close()

		for depositRows.Next() {
			var d DepositItem         // Use local type
			d.Type = "deposit"        // Set type identifier
			var depositDateStr string // Read date as string first
			var userIDIgnored int64   // Variable to scan user_id into, but ignore

			if err := depositRows.Scan(
				&d.ID,
				&userIDIgnored, // Scan UserID from DB into ignored variable
				&d.Amount,
				&d.Description,
				&depositDateStr, // Scan into string
				&d.IsRecurring,
				&d.RecurrencePeriod,
				&d.CreatedAt,
			); err != nil {
				slog.Error("failed to scan deposit row for history", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			// Parse the date string (assuming format 'YYYY-MM-DD HH:MM:SS' from DB)
			// Adjust format if DB stores it differently
			parsedDate, parseErr := time.Parse("2006-01-02 15:04:05", depositDateStr)
			if parseErr != nil {
				// Fallback or log error - maybe try just date?
				parsedDate, parseErr = time.Parse("2006-01-02", depositDateStr)
				if parseErr != nil {
					slog.Error("failed to parse deposit date from DB for history", "url", r.URL, "user_id", userID, "date_string", depositDateStr, "err", parseErr)
					d.Date = time.Time{} // Use zero time on error
				} else {
					d.Date = parsedDate
				}
			} else {
				d.Date = parsedDate
			}

			historyResponse.Deposits = append(historyResponse.Deposits, d)
		}

		if err := depositRows.Err(); err != nil {
			slog.Error("error iterating deposit rows for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: Query and append manual spendings separately if needed

		// 4. Encode and send the combined response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(historyResponse); err != nil {
			slog.Error("failed to encode history response to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
		*/
		// --- End of replaced old logic ---

		// Encode and send the response generated by the history service
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(historyResponse); err != nil {
			slog.Error("failed to encode history response to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
				s.amount,
				s.description,
				c.name AS category_name,
				u_buyer.first_name AS buyer_name,
				u_partner.first_name AS partner_name,
				us.shared_user_takes_all,
				us.shared_with -- Include shared_with ID to determine sharing status
			FROM spendings s
			JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
			JOIN user_spendings us ON s.id = us.spending_id
			JOIN categories c ON s.category = c.id
			JOIN users u_buyer ON us.buyer = u_buyer.id -- Buyer of the specific spending (should match job buyer)
			LEFT JOIN users u_partner ON us.shared_with = u_partner.id
			WHERE acs.job_id = ?
			ORDER BY s.id ASC; -- Order spendings within a job consistently
		`
		spendingStmt, err := db.Prepare(spendingQuery)
		if err != nil {
			slog.Error("failed to prepare spending query statement for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer spendingStmt.Close()

		// 2. Iterate through jobs and fetch associated spendings
		for jobRows.Next() {
			var group TransactionGroup
			group.Type = "spending_group"      // Set type identifier
			var ambiguityReason sql.NullString // Use sql.NullString for nullable reason

			if err := jobRows.Scan(
				&group.JobID,
				&group.Prompt,
				&group.TotalAmount,
				&group.Date, // Scan into the common 'Date' field
				&group.IsAmbiguityFlagged,
				&ambiguityReason,
				&group.BuyerName,
			); err != nil {
				slog.Error("failed to scan AI job row for history", "url", r.URL, "user_id", userID, "err", err)
				// Log and continue to next job? Or fail request? Let's fail for now.
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			if ambiguityReason.Valid {
				group.AmbiguityFlagReason = &ambiguityReason.String
			} else {
				group.AmbiguityFlagReason = nil
			}

			// Fetch spendings for this job ID
			spendingRows, err := spendingStmt.Query(group.JobID)
			if err != nil {
				slog.Error("failed to query spendings for job", "url", r.URL, "user_id", userID, "job_id", group.JobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			// Ensure spendingRows is closed after the inner loop finishes
			// Do NOT defer here, as it's inside the outer jobRows loop.
			// defer spendingRows.Close() // Close rows associated with this specific job <-- REMOVED DEFER

			group.Spendings = []types.SpendingItem{} // Initialize slice, use types.SpendingItem
			for spendingRows.Next() {
				var item types.SpendingItem // Use types.SpendingItem
				var partnerName sql.NullString
				var sharedWithID sql.NullInt64

				if err := spendingRows.Scan(
					&item.ID,
					&item.Amount,
					&item.Description,
					&item.CategoryName,
					&item.BuyerName,
					&partnerName,
					&item.SharedUserTakesAll,
					&sharedWithID,
				); err != nil {
					slog.Error("failed to scan spending item row for history", "url", r.URL, "user_id", userID, "job_id", group.JobID, "err", err)
					spendingRows.Close() // Close inner rows before returning
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}

				// Set partner name
				if partnerName.Valid {
					item.PartnerName = &partnerName.String
				} else {
					item.PartnerName = nil
				}

				// Determine Sharing Status for the item
				if !sharedWithID.Valid {
					item.SharingStatus = "Alone"
				} else if item.SharedUserTakesAll {
					if item.PartnerName != nil {
						item.SharingStatus = "Paid by " + *item.PartnerName
					} else {
						item.SharingStatus = "Paid by Partner"
					}
				} else {
					if item.PartnerName != nil {
						item.SharingStatus = "Shared with " + *item.PartnerName
					} else {
						item.SharingStatus = "Shared"
					}
				}
				group.Spendings = append(group.Spendings, item)
			}
			spendingRows.Close() // Explicitly close rows after iterating

			// Check for errors during spending iteration
			if err := spendingRows.Err(); err != nil {
				slog.Error("error iterating spending item rows for history", "url", r.URL, "user_id", userID, "job_id", group.JobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			historyResponse.SpendingGroups = append(historyResponse.SpendingGroups, group)
		} // End of jobRows loop

		// Check for errors from iterating over job rows.
		if err := jobRows.Err(); err != nil {
			slog.Error("error iterating AI job rows for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 3. Fetch Deposits for the user
		depositQuery := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at
			FROM deposits
			WHERE user_id = ?
			ORDER BY deposit_date DESC, created_at DESC;
		`
		depositRows, err := db.Query(depositQuery, userID)
		if err != nil {
			slog.Error("failed to query deposits for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer depositRows.Close()

		for depositRows.Next() {
			var d DepositItem         // Use local type
			d.Type = "deposit"        // Set type identifier
			var depositDateStr string // Read date as string first
			var userIDIgnored int64   // Variable to scan user_id into, but ignore

			if err := depositRows.Scan(
				&d.ID,
				&userIDIgnored, // Scan UserID from DB into ignored variable
				&d.Amount,
				&d.Description,
				&depositDateStr, // Scan into string
				&d.IsRecurring,
				&d.RecurrencePeriod,
				&d.CreatedAt,
			); err != nil {
				slog.Error("failed to scan deposit row for history", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			// Parse the date string (assuming format 'YYYY-MM-DD HH:MM:SS' from DB)
			// Adjust format if DB stores it differently
			parsedDate, parseErr := time.Parse("2006-01-02 15:04:05", depositDateStr)
			if parseErr != nil {
				// Fallback or log error - maybe try just date?
				parsedDate, parseErr = time.Parse("2006-01-02", depositDateStr)
				if parseErr != nil {
					slog.Error("failed to parse deposit date from DB for history", "url", r.URL, "user_id", userID, "date_string", depositDateStr, "err", parseErr)
					d.Date = time.Time{} // Use zero time on error
				} else {
					d.Date = parsedDate
				}
			} else {
				d.Date = parsedDate
			}

			historyResponse.Deposits = append(historyResponse.Deposits, d)
		}

		if err := depositRows.Err(); err != nil {
			slog.Error("error iterating deposit rows for history", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: Query and append manual spendings separately if needed

		// 4. Encode and send the combined response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(historyResponse); err != nil {
			slog.Error("failed to encode history response to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
	}
}

// HandleDeleteAIJob handles the deletion of an AI categorization job and its associated spendings.
func HandleDeleteAIJob(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for deleting AI job", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// 1. Get Job ID from path
		jobIDStr := r.PathValue("job_id")
		jobID, err := strconv.ParseInt(jobIDStr, 10, 64)
		if err != nil {
			slog.Warn("invalid job ID format for deletion", "url", r.URL, "user_id", userID, "job_id_str", jobIDStr, "err", err)
			http.Error(w, "Invalid job ID", http.StatusBadRequest)
			return
		}

		// 2. Begin Transaction
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for deleting AI job", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Rollback on error

		// 3. Verify Ownership: Check if the user is the buyer of this job
		var buyerID int64
		err = tx.QueryRow("SELECT buyer FROM ai_categorization_jobs WHERE id = ?", jobID).Scan(&buyerID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("delete AI job attempt on non-existent job", "url", r.URL, "user_id", userID, "job_id", jobID)
				http.Error(w, "Job not found", http.StatusNotFound)
			} else {
				slog.Error("failed to query buyer for AI job", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if buyerID != userID {
			slog.Warn("unauthorized attempt to delete AI job", "url", r.URL, "user_id", userID, "job_id", jobID, "actual_buyer_id", buyerID)
			http.Error(w, "Forbidden: You can only delete your own jobs", http.StatusForbidden)
			return
		}

		// 4. Find associated spending IDs
		spendingIDs := []int64{}
		rows, err := tx.Query("SELECT spending_id FROM ai_categorized_spendings WHERE job_id = ?", jobID)
		if err != nil {
			slog.Error("failed to query spending IDs for job deletion", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var spendingID int64
			if err := rows.Scan(&spendingID); err != nil {
				slog.Error("failed to scan spending ID during job deletion", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			spendingIDs = append(spendingIDs, spendingID)
		}
		if err := rows.Err(); err != nil {
			slog.Error("error iterating spending IDs during job deletion", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 5. Delete associated data (if any spendings found)
		if len(spendingIDs) > 0 {
			// Build placeholders for IN clause
			placeholders := make([]string, len(spendingIDs))
			args := make([]interface{}, len(spendingIDs))
			for i, id := range spendingIDs {
				placeholders[i] = "?"
				args[i] = id
			}
			inClause := strings.Join(placeholders, ",")

			// Delete from user_spendings
			userSpendingsQuery := fmt.Sprintf("DELETE FROM user_spendings WHERE spending_id IN (%s)", inClause)
			_, err = tx.Exec(userSpendingsQuery, args...)
			if err != nil {
				slog.Error("failed to delete from user_spendings during job deletion", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Delete from spendings
			spendingsQuery := fmt.Sprintf("DELETE FROM spendings WHERE id IN (%s)", inClause)
			_, err = tx.Exec(spendingsQuery, args...)
			if err != nil {
				slog.Error("failed to delete from spendings during job deletion", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			// Note: ai_categorized_spendings will be deleted by cascade when the job is deleted.
		}

		// 6. Delete the job itself
		_, err = tx.Exec("DELETE FROM ai_categorization_jobs WHERE id = ?", jobID)
		if err != nil {
			// This shouldn't fail if the ownership check passed, but handle defensively
			slog.Error("failed to delete from ai_categorization_jobs", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 7. Commit Transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for deleting AI job", "url", r.URL, "user_id", userID, "job_id", jobID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("AI job and associated spendings deleted successfully", "url", r.URL, "user_id", userID, "job_id", jobID)
		w.WriteHeader(http.StatusNoContent) // Send 204 No Content on success
	}
}

// HandleUpdateSpending allows updating the description, category, and sharing status of a specific spending item.
func HandleUpdateSpending(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for updating spending", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// 1. Get Spending ID from path
		spendingIDStr := r.PathValue("spending_id")
		spendingID, err := strconv.ParseInt(spendingIDStr, 10, 64)
		if err != nil {
			slog.Warn("invalid spending ID format", "url", r.URL, "user_id", userID, "spending_id_str", spendingIDStr, "err", err)
			http.Error(w, "Invalid spending ID", http.StatusBadRequest)
			return
		}

		// 2. Decode Payload
		var payload types.UpdateSpendingPayload // Use types.UpdateSpendingPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode update spending request body", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 3. Validate Payload
		if payload.CategoryName == "" {
			slog.Warn("invalid update spending payload: missing category", "url", r.URL, "user_id", userID, "spending_id", spendingID, "payload", payload)
			http.Error(w, "Bad Request: Category name is required", http.StatusBadRequest)
			return
		}
		isValidStatus := false
		// Use constants from types package
		validStatuses := []types.EditableSharingStatus{types.StatusAlone, types.StatusShared, types.StatusPaidByPartner}
		for _, s := range validStatuses {
			if payload.SharingStatus == s {
				isValidStatus = true
				break
			}
		}
		if !isValidStatus {
			slog.Warn("invalid update spending payload: invalid sharing status", "url", r.URL, "user_id", userID, "spending_id", spendingID, "payload", payload)
			http.Error(w, "Bad Request: Invalid sharing status", http.StatusBadRequest)
			return
		}

		// 4. Begin Transaction
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for update spending", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Rollback on error

		// 5. Verify Authorization: Check if the user is the buyer of this spending item
		var buyerID int64
		err = tx.QueryRow("SELECT buyer FROM user_spendings WHERE spending_id = ?", spendingID).Scan(&buyerID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("update spending attempt on non-existent spending or user_spending link", "url", r.URL, "user_id", userID, "spending_id", spendingID)
				http.Error(w, "Spending item not found", http.StatusNotFound)
			} else {
				slog.Error("failed to query buyer for spending item", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		if buyerID != userID {
			slog.Warn("unauthorized attempt to update spending item", "url", r.URL, "user_id", userID, "spending_id", spendingID, "actual_buyer_id", buyerID)
			http.Error(w, "Forbidden: You can only edit your own spending items", http.StatusForbidden)
			return
		}

		// 6. Get Category ID
		var categoryID int64
		err = tx.QueryRow("SELECT id FROM categories WHERE name = ?", payload.CategoryName).Scan(&categoryID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("category not found during update spending", "url", r.URL, "user_id", userID, "spending_id", spendingID, "category_name", payload.CategoryName)
				http.Error(w, "Category not found", http.StatusBadRequest) // Bad request because category name is invalid
			} else {
				slog.Error("failed to query category ID during update spending", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		// 7. Determine new user_spendings values based on SharingStatus
		var sharedWithID *int64 = nil
		var sharedUserTakesAll bool = false
		var partnerID int64
		partnerExists := false

		// Use constants from types package
		if payload.SharingStatus == types.StatusShared || payload.SharingStatus == types.StatusPaidByPartner {
			// Need partner ID for these statuses
			// Use the new GetPartnerUserID which queries the DB
			fetchedPartnerID, ok := auth.GetPartnerUserID(tx, userID) // Pass transaction tx
			if !ok {
				// Check if the status requires a partner
				// Use constants from types package
				if payload.SharingStatus == types.StatusShared || payload.SharingStatus == types.StatusPaidByPartner {
					// GetPartnerUserID logs errors
					slog.Warn("attempted to set sharing status requiring partner, but no partner configured", "url", r.URL, "user_id", userID, "spending_id", spendingID, "status", payload.SharingStatus)
					http.Error(w, "Cannot set status to 'Shared' or 'Paid by Partner': No partner configured for your user.", http.StatusBadRequest)
					return
				}
				// If status is 'Alone', it's fine that no partner exists.
			} else {
				partnerID = fetchedPartnerID
				partnerExists = true // Assume partner exists if GetPartnerUserID returns one
				// Optional: Verify partner exists in DB? For simplicity, we trust GetPartnerUserID for now.
			}

			if partnerExists {
				sharedWithID = &partnerID
				// Use constant from types package
				if payload.SharingStatus == types.StatusPaidByPartner {
					sharedUserTakesAll = true
				}
			}
		}
		// If payload.SharingStatus is types.StatusAlone, sharedWithID remains nil and sharedUserTakesAll remains false.

		// 8. Update spendings table
		_, err = tx.Exec(`UPDATE spendings SET description = ?, category = ? WHERE id = ?`,
			payload.Description, categoryID, spendingID)
		if err != nil {
			// Check if the spending item itself was deleted between checks (unlikely but possible)
			// Or if there's another DB error.
			slog.Error("failed to update spendings table", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 9. Update user_spendings table
		_, err = tx.Exec(`UPDATE user_spendings SET shared_with = ?, shared_user_takes_all = ? WHERE spending_id = ?`,
			sharedWithID, sharedUserTakesAll, spendingID)
		if err != nil {
			// This update should always find a row because we checked user_spendings in step 5.
			slog.Error("failed to update user_spendings table", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 10. Commit Transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for update spending", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Spending item updated successfully", "url", r.URL, "user_id", userID, "spending_id", spendingID)
		w.WriteHeader(http.StatusOK) // Send 200 OK on success
	}
}
