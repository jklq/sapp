package transfer

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/types" // Import shared types
)

// HandleGetTransferStatus calculates and returns the net balance between the user and their partner.
func HandleGetTransferStatus(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for transfer status", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Use the new GetPartnerUserID which queries the DB
		partnerID, partnerOk := auth.GetPartnerUserID(db, userID) // Pass db connection
		if !partnerOk {
			// GetPartnerUserID logs errors, just return appropriate response
			// slog.Warn("no partner configured for user, cannot calculate transfer status", "url", r.URL, "user_id", userID)
			http.Error(w, "Partner not found or not configured for this user.", http.StatusBadRequest) // More specific error
			return
		}

		// Get user and partner names
		var userName, partnerName string
		err := db.QueryRow("SELECT first_name FROM users WHERE id = ?", userID).Scan(&userName)
		if err != nil {
			slog.Error("failed to query user name", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		err = db.QueryRow("SELECT first_name FROM users WHERE id = ?", partnerID).Scan(&partnerName)
		if err != nil {
			// Log error but continue, partner might have been deleted?
			slog.Error("failed to query partner name", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
			partnerName = "Partner" // Fallback name
		}

		// Calculate net balance from unsettled items
		query := `
            SELECT
                us.buyer, us.shared_with, us.shared_user_takes_all, s.amount
            FROM user_spendings us
            JOIN spendings s ON us.spending_id = s.id
            WHERE us.settled_at IS NULL
              AND ( (us.buyer = ? AND us.shared_with = ?) OR (us.buyer = ? AND us.shared_with = ?) )
        `
		rows, err := db.Query(query, userID, partnerID, partnerID, userID)
		if err != nil {
			slog.Error("failed to query unsettled spendings for transfer status", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		userNetBalance := 0.0
		for rows.Next() {
			var buyer, sharedWith sql.NullInt64 // Use NullInt64 for shared_with
			var sharedUserTakesAll bool
			var amount float64

			if err := rows.Scan(&buyer, &sharedWith, &sharedUserTakesAll, &amount); err != nil {
				slog.Error("failed to scan spending row for transfer status", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}

			// Ensure buyer is valid before proceeding
			if !buyer.Valid {
				slog.Warn("Skipping row with NULL buyer in user_spendings", "url", r.URL, "user_id", userID)
				continue // Should not happen based on schema constraints
			}

			if buyer.Int64 == userID {
				// User paid
				if sharedWith.Valid && sharedWith.Int64 == partnerID {
					// Shared with partner
					if sharedUserTakesAll {
						userNetBalance += amount // Partner owes full amount
					} else {
						userNetBalance += amount / 2.0 // Partner owes half
					}
				}
				// If sharedWith is NULL or not partner, it doesn't affect the balance
			} else if buyer.Int64 == partnerID {
				// Partner paid
				if sharedWith.Valid && sharedWith.Int64 == userID {
					// Shared with user
					if sharedUserTakesAll {
						userNetBalance -= amount // User owes full amount
					} else {
						userNetBalance -= amount / 2.0 // User owes half
					}
				}
				// If sharedWith is NULL or not user, it doesn't affect the balance
			}
		}

		if err := rows.Err(); err != nil {
			slog.Error("error iterating unsettled spending rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Determine response fields based on balance
		resp := types.TransferStatusResponse{ // Use types.TransferStatusResponse
			PartnerName: partnerName,
			AmountOwed:  math.Abs(userNetBalance), // Always positive amount
			OwedBy:      nil,
			OwedTo:      nil,
		}

		// Use a small tolerance for floating point comparison
		tolerance := 0.001 // Less than a cent

		if userNetBalance > tolerance { // Partner owes user
			resp.OwedBy = &partnerName
			resp.OwedTo = &userName
		} else if userNetBalance < -tolerance { // User owes partner
			resp.OwedBy = &userName
			resp.OwedTo = &partnerName
		} else {
			// Considered settled (difference is negligible)
			resp.AmountOwed = 0 // Ensure it's exactly 0 if within tolerance
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode transfer status response", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
		}
	}
}

// HandleRecordTransfer marks relevant spendings as settled and records the transfer event.
func HandleRecordTransfer(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for recording transfer", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Use the new GetPartnerUserID which queries the DB
		partnerID, partnerOk := auth.GetPartnerUserID(db, userID) // Pass db connection
		if !partnerOk {
			// GetPartnerUserID logs errors
			// slog.Warn("no partner configured for user, cannot record transfer", "url", r.URL, "user_id", userID)
			http.Error(w, "Partner not found or not configured for this user.", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()

		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction for recording transfer", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Rollback on error

		// Update user_spendings to mark as settled
		updateQuery := `
            UPDATE user_spendings
            SET settled_at = ?
            WHERE settled_at IS NULL
              AND ( (buyer = ? AND shared_with = ?) OR (buyer = ? AND shared_with = ?) )
        `
		_, err = tx.Exec(updateQuery, now, userID, partnerID, partnerID, userID)
		if err != nil {
			slog.Error("failed to update user_spendings during transfer recording", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Insert into transfers table
		insertQuery := `
            INSERT INTO transfers (settled_by_user_id, settled_with_user_id, settlement_time)
            VALUES (?, ?, ?)
        `
		_, err = tx.Exec(insertQuery, userID, partnerID, now)
		if err != nil {
			slog.Error("failed to insert into transfers table", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Commit transaction
		if err = tx.Commit(); err != nil {
			slog.Error("failed to commit transaction for recording transfer", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		slog.Info("Transfer recorded successfully", "url", r.URL, "user_id", userID, "partner_id", partnerID)
		w.WriteHeader(http.StatusOK) // Send 200 OK on success
	}
}
