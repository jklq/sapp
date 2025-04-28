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

// parseDateParam parses a date string ("YYYY-MM-DD") from query parameters.
// Returns zero time and error if parsing fails.
func parseDateParam(r *http.Request, paramName string) (time.Time, error) {
	dateStr := r.URL.Query().Get(paramName)
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("missing query parameter: %s", paramName)
	}
	parsedDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format for %s (use YYYY-MM-DD): %w", paramName, err)
	}
	// Return the date at the beginning of the day in UTC
	return time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC), nil
}

// HandleGetSpendingStats calculates and returns spending totals per category for a given date range.
// Expects "startDate" and "endDate" query parameters in "YYYY-MM-DD" format.
func HandleGetSpendingStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for stats", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Parse startDate and endDate from query parameters
		startDate, err := parseDateParam(r, "startDate")
		if err != nil {
			slog.Warn("Failed to parse startDate", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		endDate, err := parseDateParam(r, "endDate")
		if err != nil {
			slog.Warn("Failed to parse endDate", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Ensure endDate is not before startDate
		if endDate.Before(startDate) {
			slog.Warn("Invalid date range: endDate is before startDate", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate)
			http.Error(w, "Bad Request: endDate cannot be before startDate", http.StatusBadRequest)
			return
		}
		// Adjust endDate to the end of the day to include all spendings on that day
		endDateEndOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)


		// Query to sum spending amounts per category for the user within the specified date range.
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
                s.spending_date >= ? AND s.spending_date <= ? -- Use date range
                AND (us.buyer = ? OR us.shared_with = ?) -- Include spendings where user is buyer OR shared_with
            GROUP BY c.name
            HAVING total_amount > 0.001 -- Only include categories with spending (use tolerance)
            ORDER BY total_amount DESC;
        `

		rows, err := db.Query(query,
			userID, userID, userID, // Conditions for when user is the buyer
			userID, userID, // Conditions for when user is shared_with
			userID, userID, // Conditions for when user is shared_with (paid by partner)
			startDate.Format(time.RFC3339),     // Start date condition
			endDateEndOfDay.Format(time.RFC3339), // End date condition (end of day)
			userID, userID,                     // Filter condition for relevant user_spendings rows
		)

		if err != nil {
			slog.Error("failed to query spending stats", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate, "err", err)
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
			// Round to 2 decimal places before appending
			stat.TotalAmount = math.Round(stat.TotalAmount*100) / 100
			stats = append(stats, stat)
		}

		if err := rows.Err(); err != nil {
			slog.Error("error iterating spending stat rows", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Return empty array instead of null if no stats found
		if stats == nil {
			stats = []types.CategorySpendingStat{}
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(stats); err != nil {
			slog.Error("failed to encode spending stats to JSON", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate, "err", err)
		}
	}
}
