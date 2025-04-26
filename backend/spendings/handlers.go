package spendings

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
)

// SpendingDetail represents the detailed structure of a spending item returned by the API.
type SpendingDetail struct {
	ID                  int64      `json:"id"`
	Amount              float64    `json:"amount"`
	Description         string     `json:"description"`
	CategoryName        string     `json:"category_name"`
	CreatedAt           time.Time  `json:"created_at"`
	BuyerName           string     `json:"buyer_name"`
	PartnerName         *string    `json:"partner_name"`         // Pointer to handle NULL
	SharedUserTakesAll  bool       `json:"shared_user_takes_all"` // Indicates if partner pays all
	SharingStatus       string     `json:"sharing_status"`       // Derived: "Alone", "Shared", "Paid by Partner"
}

// HandleGetSpendings returns an http.HandlerFunc that fetches all spendings for the logged-in user.
func HandleGetSpendings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting spendings", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// 1. Fetch AI Categorization Jobs initiated by the user
		jobQuery := `
			SELECT
				j.id, j.prompt, j.total_amount, j.created_at, j.is_ambiguity_flagged, j.ambiguity_flag_reason
			FROM ai_categorization_jobs j
			WHERE j.buyer = ?
			ORDER BY j.created_at DESC;
		`
		jobRows, err := db.Query(jobQuery, userID)
		if err != nil {
			slog.Error("failed to query AI categorization jobs", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer jobRows.Close()

		transactionGroups := []TransactionGroup{}

		// Prepare statement for fetching spendings for a specific job ID
		// This avoids N+1 query problem inside the loop
		spendingQuery := `
			SELECT
				s.id,
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
			slog.Error("failed to prepare spending query statement", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer spendingStmt.Close()

		// 2. Iterate through jobs and fetch associated spendings
		for jobRows.Next() {
			var group TransactionGroup
			var ambiguityReason sql.NullString // Use sql.NullString for nullable reason

			if err := jobRows.Scan(
				&group.JobID,
				&group.Prompt,
				&group.TotalAmount,
				&group.JobCreatedAt,
				&group.IsAmbiguityFlagged,
				&ambiguityReason,
			); err != nil {
				slog.Error("failed to scan AI job row", "url", r.URL, "user_id", userID, "err", err)
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

			group.Spendings = []SpendingItem{} // Initialize slice
			for spendingRows.Next() {
				var item SpendingItem
				var partnerName sql.NullString
				var sharedWithID sql.NullInt64

				if err := spendingRows.Scan(
					&item.ID,
					&item.Amount,
					&item.Description,
					&item.CategoryName,
					&item.BuyerName, // This should be the same as the job buyer, but we fetch it per spending item
					&partnerName,
					&item.SharedUserTakesAll,
					&sharedWithID,
				); err != nil {
					slog.Error("failed to scan spending item row", "url", r.URL, "user_id", userID, "job_id", group.JobID, "err", err)
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
			spendingRows.Close() // Close inner rows after loop

			// Check for errors during spending iteration
			if err := spendingRows.Err(); err != nil {
				slog.Error("error iterating spending item rows", "url", r.URL, "user_id", userID, "job_id", group.JobID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			transactionGroups = append(transactionGroups, group)
		} // End of jobRows loop

		// Check for errors from iterating over job rows.
		if err := jobRows.Err(); err != nil {
			slog.Error("error iterating AI job rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// TODO: Query and append manual spendings separately if needed

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(transactionGroups); err != nil {
			slog.Error("failed to encode transaction groups to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
	}
}
