package stats

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/types"
)

// HandleGetLastMonthSpendingStats calculates and returns spending totals per category for the last 30 days.
func HandleGetLastMonthSpendingStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for stats", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Calculate the date 30 days ago from now
		thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)

		// Query to sum spending amounts per category for the user within the last 30 days.
		// This query considers the user's share of the cost based on user_spendings.
		// - If buyer=userID and shared_with is NULL -> user pays full amount.
		// - If buyer=userID and shared_with=partnerID and shared_user_takes_all=false -> user pays half.
		// - If buyer=userID and shared_with=partnerID and shared_user_takes_all=true -> user pays zero (partner pays all).
		// - If buyer=partnerID and shared_with=userID and shared_user_takes_all=false -> user pays half.
		// - If buyer=partnerID and shared_with=userID and shared_user_takes_all=true -> user pays full amount.
		query := `
            SELECT
                c.name AS category_name,
                SUM(
                    CASE
                        -- User paid, not shared
                        WHEN us.buyer = ? AND us.shared_with IS NULL THEN s.amount
                        -- User paid, shared 50/50
                        WHEN us.buyer = ? AND us.shared_with IS NOT NULL AND us.shared_user_takes_all = 0 THEN s.amount / 2.0
                        -- User paid, partner pays all
                        WHEN us.buyer = ? AND us.shared_with IS NOT NULL AND us.shared_user_takes_all = 1 THEN 0.0
                        -- Partner paid, shared 50/50
                        WHEN us.buyer != ? AND us.shared_with = ? AND us.shared_user_takes_all = 0 THEN s.amount / 2.0
                        -- Partner paid, user pays all
                        WHEN us.buyer != ? AND us.shared_with = ? AND us.shared_user_takes_all = 1 THEN s.amount
                        ELSE 0.0 -- Should not happen with current logic, but default to 0
                    END
                ) AS total_amount
            FROM spendings s
            JOIN categories c ON s.category = c.id
            JOIN user_spendings us ON s.id = us.spending_id
            WHERE
                s.spending_date >= ?
                AND (us.buyer = ? OR us.shared_with = ?) -- Include spendings where user is buyer OR shared_with
            GROUP BY c.name
            HAVING total_amount > 0 -- Only include categories with spending
            ORDER BY total_amount DESC;
        `

		rows, err := db.Query(query,
			userID, userID, userID, // Conditions for when user is the buyer
			userID, userID, // Conditions for when user is shared_with
			userID, userID, // Conditions for when user is shared_with (paid by partner)
			thirtyDaysAgo.Format(time.RFC3339), // Date condition
			userID, userID,                     // Filter condition for relevant user_spendings rows
		)

		if err != nil {
			slog.Error("failed to query spending stats", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		stats := []types.CategorySpendingStat{} // Use types.CategorySpendingStat
		for rows.Next() {
			var stat types.CategorySpendingStat // Use types.CategorySpendingStat
			if err := rows.Scan(&stat.CategoryName, &stat.TotalAmount); err != nil {
				slog.Error("failed to scan spending stat row", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			stats = append(stats, stat)
		}

		if err := rows.Err(); err != nil {
			slog.Error("error iterating spending stat rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			slog.Error("failed to encode spending stats to JSON", "url", r.URL, "user_id", userID, "err", err)
		}
	}
}
