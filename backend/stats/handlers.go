package stats

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
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
			startDate.Format(time.RFC3339),       // Start date condition
			endDateEndOfDay.Format(time.RFC3339), // End date condition (end of day)
			userID, userID,                       // Filter condition for relevant user_spendings rows
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

// --- Deposit Stats Helpers (Adapted from history service) ---

// calculateNextDate calculates the next occurrence date based on the period.
// TODO: Consider moving this to a shared utility or the deposit package.
func calculateNextDate(current time.Time, period string) time.Time {
	switch period {
	case "weekly":
		return current.AddDate(0, 0, 7)
	case "monthly":
		return current.AddDate(0, 1, 0)
	case "yearly":
		return current.AddDate(1, 0, 0)
	default:
		slog.Warn("unsupported recurrence period encountered in stats calculation", "period", period)
		return time.Time{} // Return zero time for unsupported periods
	}
}

// generateDepositOccurrencesInRange calculates occurrence dates for a deposit template
// that fall within the specified start and end dates.
// TODO: Consider moving this to a shared utility or the deposit package.
func generateDepositOccurrencesInRange(template types.Deposit, startDate, endDate time.Time) []time.Time {
	occurrences := []time.Time{}
	currentDate := template.DepositDate // Start from the initial deposit date

	// Determine the effective end date for generation: the earlier of the template's end_date or the query's endDate.
	effectiveEndDate := endDate
	if template.EndDate != nil && template.EndDate.Before(endDate) {
		effectiveEndDate = *template.EndDate
	}

	// Iterate from the template start date
	for {
		// Stop if the current date is after the effective end date
		if currentDate.After(effectiveEndDate) {
			break
		}

		// Add the occurrence if it's within the query's date range [startDate, endDate]
		// Note: startDate comparison uses !Before to include the start date itself.
		if !currentDate.Before(startDate) {
			occurrences = append(occurrences, currentDate)
		}

		// If not recurring, we are done after checking the initial date
		if !template.IsRecurring || template.RecurrencePeriod == nil {
			break
		}

		// Calculate the next date
		next := calculateNextDate(currentDate, *template.RecurrencePeriod)
		// Stop if next date is invalid or the same as current (shouldn't happen with AddDate)
		if next.IsZero() || !next.After(currentDate) {
			slog.Warn("Recurring deposit calculation resulted in zero or non-advancing date", "template_id", template.ID, "current_date", currentDate, "period", *template.RecurrencePeriod)
			break
		}
		currentDate = next
	}

	return occurrences
}

// --- End Deposit Stats Helpers ---

// HandleGetDepositStats calculates and returns the total deposit amount for a given date range.
// Expects "startDate" and "endDate" query parameters in "YYYY-MM-DD" format.
func HandleGetDepositStats(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for deposit stats", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		// Parse startDate and endDate from query parameters
		startDate, err := parseDateParam(r, "startDate")
		if err != nil {
			slog.Warn("Failed to parse startDate for deposit stats", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		endDate, err := parseDateParam(r, "endDate")
		if err != nil {
			slog.Warn("Failed to parse endDate for deposit stats", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Ensure endDate is not before startDate
		if endDate.Before(startDate) {
			slog.Warn("Invalid date range for deposit stats: endDate is before startDate", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate)
			http.Error(w, "Bad Request: endDate cannot be before startDate", http.StatusBadRequest)
			return
		}
		// Adjust endDate to the end of the day to include all deposits on that day
		endDateEndOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

		// 1. Fetch all deposit templates for the user
		templates := []types.Deposit{} // Use types.Deposit which includes EndDate
		query := `
			SELECT id, user_id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at
			FROM deposits
			WHERE user_id = ?
		`
		rows, err := db.Query(query, userID)
		if err != nil {
			slog.Error("failed to query deposit templates for stats", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var d types.Deposit
			var endDateDB sql.NullTime
			var recurrencePeriodDB sql.NullString

			// Scan directly into time.Time for DepositDate and CreatedAt
			if err := rows.Scan(
				&d.ID, &d.UserID, &d.Amount, &d.Description, &d.DepositDate,
				&d.IsRecurring, &recurrencePeriodDB, &endDateDB, &d.CreatedAt,
			); err != nil {
				slog.Error("failed to scan deposit template row for stats", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return // Important to return here
			}
			// Convert nullable fields
			if recurrencePeriodDB.Valid {
				d.RecurrencePeriod = &recurrencePeriodDB.String
			}
			if endDateDB.Valid {
				d.EndDate = &endDateDB.Time
			}
			templates = append(templates, d)
		}
		if err = rows.Err(); err != nil {
			slog.Error("error iterating deposit template rows for stats", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// 2. Calculate total amount from relevant occurrences
		totalAmount := 0.0
		depositCount := 0
		for _, template := range templates {
			if template.IsRecurring && template.RecurrencePeriod != nil {
				// Generate occurrences within the requested range [startDate, endDateEndOfDay]
				occurrences := generateDepositOccurrencesInRange(template, startDate, endDateEndOfDay)
				for range occurrences {
					totalAmount += template.Amount
					depositCount++
				}
			} else {
				// Non-recurring: check if the single deposit_date falls within the range
				// Note: startDate comparison uses !Before to include the start date itself.
				if !template.DepositDate.Before(startDate) && !template.DepositDate.After(endDateEndOfDay) {
					totalAmount += template.Amount
					depositCount++
				}
			}
		}

		// Round final amount
		totalAmount = math.Round(totalAmount*100) / 100

		// 3. Prepare and send response
		resp := types.DepositStatsResponse{
			TotalAmount: totalAmount,
			Count:       depositCount,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			slog.Error("failed to encode deposit stats to JSON", "url", r.URL, "user_id", userID, "startDate", startDate, "endDate", endDate, "err", err)
		}
	}
}
