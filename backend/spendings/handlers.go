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

		// Query to fetch spendings involving the user, either as buyer or shared_with partner
		// We join necessary tables to get names and sharing details.
		// We need to fetch spendings where the user is the buyer OR the shared_with partner.
		query := `
            SELECT
                s.id,
                s.amount,
                s.description,
                c.name AS category_name,
                s.created_at,
                u_buyer.first_name AS buyer_name,
                u_partner.first_name AS partner_name, -- Use COALESCE or handle NULL in Scan
                us.shared_user_takes_all,
				us.buyer, -- Include buyer ID to determine context
				us.shared_with -- Include shared_with ID to determine context
            FROM spendings s
            JOIN user_spendings us ON s.id = us.spending_id
            JOIN categories c ON s.category = c.id
            JOIN users u_buyer ON us.buyer = u_buyer.id
            LEFT JOIN users u_partner ON us.shared_with = u_partner.id -- LEFT JOIN for partner
            WHERE us.buyer = ? OR us.shared_with = ?
            ORDER BY s.created_at DESC;
        `

		rows, err := db.Query(query, userID, userID)
		if err != nil {
			slog.Error("failed to query spendings", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		spendings := []SpendingDetail{}
		for rows.Next() {
			var detail SpendingDetail
			var partnerName sql.NullString // Use sql.NullString for nullable partner name
			var buyerID int64
			var sharedWithID sql.NullInt64 // Use sql.NullInt64 for nullable shared_with ID

			if err := rows.Scan(
				&detail.ID,
				&detail.Amount,
				&detail.Description,
				&detail.CategoryName,
				&detail.CreatedAt,
				&detail.BuyerName,
				&partnerName, // Scan into sql.NullString
				&detail.SharedUserTakesAll,
				&buyerID,
				&sharedWithID,
			); err != nil {
				slog.Error("failed to scan spending row", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return // Important to return here to avoid processing partial data
			}

			// Set partner name if valid
			if partnerName.Valid {
				detail.PartnerName = &partnerName.String
			} else {
				detail.PartnerName = nil
			}

			// Determine Sharing Status based on scanned data
			if !sharedWithID.Valid {
				detail.SharingStatus = "Alone" // No partner involved
			} else if detail.SharedUserTakesAll {
				// Partner exists and takes all cost
				if detail.PartnerName != nil {
					detail.SharingStatus = "Paid by " + *detail.PartnerName
				} else {
					detail.SharingStatus = "Paid by Partner" // Fallback if name is missing
				}
			} else {
				// Partner exists and cost is shared
				if detail.PartnerName != nil {
					detail.SharingStatus = "Shared with " + *detail.PartnerName
				} else {
					detail.SharingStatus = "Shared" // Fallback if name is missing
				}
			}


			spendings = append(spendings, detail)
		}

		// Check for errors from iterating over rows.
		if err := rows.Err(); err != nil {
			slog.Error("error iterating spending rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(spendings); err != nil {
			slog.Error("failed to encode spendings to JSON", "url", r.URL, "user_id", userID, "err", err)
			// Avoid writing header again if already written
			// http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}
