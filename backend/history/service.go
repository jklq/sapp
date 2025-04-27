package history

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"git.sr.ht/~relay/sapp-backend/types" // Import shared types
)

// HistoryListItem represents a generic item in the combined history list for internal processing.
// It includes common fields for sorting and identification, and the raw item.
type HistoryListItem struct {
	Type     string    `json:"type"` // "spending_group" or "deposit"
	Date     time.Time `json:"date"` // Primary sorting key (job creation time or deposit occurrence date)
	RawItem interface{} `json:"-"` // Store the original struct (types.TransactionGroup or types.DepositItem), ignored by JSON
	// Add other common fields if necessary
}

// GenerateHistory fetches and combines spending groups and deposit occurrences for a user up to a given end date.
func GenerateHistory(db *sql.DB, userID int64, endDate time.Time) ([]HistoryListItem, error) {
	// We'll fetch all relevant items and generate occurrences up to the endDate.
	// For simplicity, we won't implement a startDate filter yet, but it could be added.

	allHistoryItems := []HistoryListItem{}
	// now := time.Now().UTC() // Use consistent time for generation limit - REMOVED, use endDate parameter

	// --- 1. Fetch Spending Groups (Transaction Groups) ---
	spendingGroups, err := fetchSpendingGroups(db, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spending groups: %w", err)
	}
	for _, group := range spendingGroups {
		// Ensure group is copied to avoid loop variable issues if using pointers later
		g := group
		allHistoryItems = append(allHistoryItems, HistoryListItem{
			Type:    "spending_group",
			Date:    g.Date, // Use job creation time for sorting
			RawItem: g,
		})
	}

	// --- 2. Fetch Deposits (Recurring Templates and Non-Recurring) ---
	deposits, err := fetchDeposits(db, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch deposits: %w", err)
	}

	// --- 3. Generate Occurrences for Recurring Deposits ---
	generatedDeposits := []types.DepositItem{} // Use types.DepositItem
	for _, deposit := range deposits {
		if deposit.IsRecurring && deposit.RecurrencePeriod != nil {
			// Generate occurrences for this recurring deposit up to the provided endDate
			occurrences := generateDepositOccurrences(deposit, endDate) // Generate up to endDate
			generatedDeposits = append(generatedDeposits, occurrences...)
		} else {
			// Add non-recurring deposits directly
			generatedDeposits = append(generatedDeposits, deposit)
		}
	}

	// --- 4. Add Generated/Non-Recurring Deposits to Combined List ---
	for _, deposit := range generatedDeposits {
		// Ensure deposit is copied
		d := deposit
		allHistoryItems = append(allHistoryItems, HistoryListItem{
			Type:    "deposit",
			Date:    d.Date, // Use the occurrence date for sorting
			RawItem: d,
		})
	}

	// --- 5. Sort Combined List by Date Descending ---
	sort.Slice(allHistoryItems, func(i, j int) bool {
		return allHistoryItems[j].Date.Before(allHistoryItems[i].Date) // j before i for descending
	})

	return allHistoryItems, nil
}

// fetchSpendingGroups fetches transaction groups (as types.TransactionGroup) initiated by the user.
func fetchSpendingGroups(db *sql.DB, userID int64) ([]types.TransactionGroup, error) {
	groups := []types.TransactionGroup{} // Use types.TransactionGroup
	jobQuery := `
		SELECT
			j.id, j.prompt, j.total_amount, j.created_at AS date, j.is_ambiguity_flagged, j.ambiguity_flag_reason, u.first_name AS buyer_name
		FROM ai_categorization_jobs j
		JOIN users u ON j.buyer = u.id
		WHERE j.buyer = ?
		ORDER BY j.created_at DESC;
	`
	jobRows, err := db.Query(jobQuery, userID)
	if err != nil {
		slog.Error("failed to query AI categorization jobs", "user_id", userID, "err", err)
		return nil, err
	}
	defer jobRows.Close()

	spendingQuery := `
		SELECT
			s.id, s.amount, s.description, c.name AS category_name,
			u_buyer.first_name AS buyer_name, u_partner.first_name AS partner_name,
			us.shared_user_takes_all, us.shared_with
		FROM spendings s
		JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
		JOIN user_spendings us ON s.id = us.spending_id
		JOIN categories c ON s.category = c.id
		JOIN users u_buyer ON us.buyer = u_buyer.id
		LEFT JOIN users u_partner ON us.shared_with = u_partner.id
		WHERE acs.job_id = ?
		ORDER BY s.id ASC;
	`
	spendingStmt, err := db.Prepare(spendingQuery)
	if err != nil {
		slog.Error("failed to prepare spending query statement for history", "user_id", userID, "err", err)
		return nil, err
	}
	defer spendingStmt.Close()

	for jobRows.Next() {
		var group types.TransactionGroup // Use types.TransactionGroup
		var ambiguityReason sql.NullString

		if err := jobRows.Scan(
			&group.JobID, &group.Prompt, &group.TotalAmount, &group.Date, // Scan directly into Date field
			&group.IsAmbiguityFlagged, &ambiguityReason, &group.BuyerName,
		); err != nil {
			slog.Error("failed to scan AI job row for history", "user_id", userID, "err", err)
			return nil, err
		}
		group.AmbiguityFlagReason = sqlNullStringToPointer(ambiguityReason)

		spendingRows, err := spendingStmt.Query(group.JobID)
		if err != nil {
			slog.Error("failed to query spendings for job", "user_id", userID, "job_id", group.JobID, "err", err)
			return nil, err
		}

		group.Spendings = []types.SpendingItem{} // Use types.SpendingItem
		for spendingRows.Next() {
			var item types.SpendingItem // Use types.SpendingItem
			var partnerName sql.NullString
			var sharedWithID sql.NullInt64

			if err := spendingRows.Scan(
				&item.ID, &item.Amount, &item.Description, &item.CategoryName,
				&item.BuyerName, &partnerName, &item.SharedUserTakesAll, &sharedWithID,
			); err != nil {
				slog.Error("failed to scan spending item row for history", "user_id", userID, "job_id", group.JobID, "err", err)
				spendingRows.Close()
				return nil, err
			}

			item.PartnerName = sqlNullStringToPointer(partnerName)
			item.SharingStatus = determineSharingStatus(sharedWithID.Valid, item.SharedUserTakesAll, item.PartnerName)
			group.Spendings = append(group.Spendings, item)
		}
		spendingRows.Close()
		if err := spendingRows.Err(); err != nil {
			slog.Error("error iterating spending item rows for history", "user_id", userID, "job_id", group.JobID, "err", err)
			return nil, err
		}
		groups = append(groups, group)
	}
	if err := jobRows.Err(); err != nil {
		slog.Error("error iterating AI job rows for history", "user_id", userID, "err", err)
		return nil, err
	}
	return groups, nil
}

// fetchDeposits fetches all deposit records (as types.DepositItem) for the user.
// fetchDeposits fetches all active deposit templates (as types.DepositItem) for the user.
func fetchDeposits(db *sql.DB, userID int64) ([]types.DepositItem, error) {
	fetchedDeposits := []types.DepositItem{} // Use types.DepositItem
	// Fetch only templates, not individual occurrences here.
	// The history service will generate occurrences based on these templates.
	depositQuery := `
		SELECT id, amount, description, deposit_date, is_recurring, recurrence_period, end_date, created_at
		FROM deposits
		WHERE user_id = ? -- Removed is_active filter, assuming hard delete for now
		ORDER BY deposit_date DESC, created_at DESC;
	`
	depositRows, err := db.Query(depositQuery, userID)
	if err != nil {
		slog.Error("failed to query deposits for history service", "user_id", userID, "err", err)
		return nil, err
	}
	defer depositRows.Close()

	for depositRows.Next() {
		var d types.DepositItem // Use types.DepositItem
		// var depositDateDB time.Time // No longer needed, scan directly into d.Date

		if err := depositRows.Scan(
			&d.ID,
			&d.Amount,
			&d.Description,
			&d.Date, // Scan directly into d.Date
			&d.IsRecurring,
			&d.RecurrencePeriod,
			&d.EndDate, // Scan the new end_date field
			&d.CreatedAt,
		); err != nil {
			slog.Error("failed to scan deposit template row for history service", "user_id", userID, "err", err)
			return nil, err
		}
		// d.Date = depositDateDB // No longer needed
		fetchedDeposits = append(fetchedDeposits, d)
	}
	if err := depositRows.Err(); err != nil {
		slog.Error("error iterating deposit rows for history service", "user_id", userID, "err", err)
		return nil, err
	}
	return fetchedDeposits, nil
}

// generateDepositOccurrences calculates occurrence dates for a deposit template up to a given limit.
// It respects the template's end_date if set.
func generateDepositOccurrences(template types.DepositItem, generationLimitDate time.Time) []types.DepositItem {
	occurrences := []types.DepositItem{} // Use types.DepositItem
	currentDate := template.Date         // Start from the initial deposit date

	// Determine the effective end date for generation: the earlier of the template's end_date or the overall generation limit.
	effectiveEndDate := generationLimitDate
	if template.EndDate != nil && template.EndDate.Before(generationLimitDate) {
		effectiveEndDate = *template.EndDate
	}

	// Ensure we don't generate occurrences before the template start date
	// and include the initial date itself if it's within the effective range.
	if currentDate.After(effectiveEndDate) {
		return occurrences // Template starts after the effective end date
	}

	// Add the initial occurrence if it's on or before the effective end date
	// (The check above already ensures currentDate <= effectiveEndDate if we reach here)
	occurrences = append(occurrences, createOccurrence(template, currentDate))

	// If not recurring, we are done after adding the initial one (if applicable)
	if !template.IsRecurring || template.RecurrencePeriod == nil {
		return occurrences
	}

	// Generate subsequent occurrences for recurring deposits
	for {
		nextDate := calculateNextDate(currentDate, *template.RecurrencePeriod)
		// Stop if next date is invalid or after the effective end date
		if nextDate.IsZero() || nextDate.After(effectiveEndDate) {
			break
		}
		occurrences = append(occurrences, createOccurrence(template, nextDate))
		currentDate = nextDate
	}

	return occurrences
}

// createOccurrence creates a types.DepositItem instance for a specific occurrence date.
func createOccurrence(template types.DepositItem, occurrenceDate time.Time) types.DepositItem {
	return types.DepositItem{
		ID:          template.ID, // Link back to the original template ID
		Amount:      template.Amount,
		Description:      template.Description,
		Date:             occurrenceDate, // This specific occurrence's date
		IsRecurring:      template.IsRecurring, // Keep original template flag
		RecurrencePeriod: template.RecurrencePeriod,
		EndDate:          template.EndDate, // Keep original template end date (might be useful info)
		CreatedAt:        template.CreatedAt, // Keep original creation time
	}
}

// calculateNextDate calculates the next occurrence date based on the period.
func calculateNextDate(current time.Time, period string) time.Time {
	switch period {
	case "weekly":
		return current.AddDate(0, 0, 7)
	case "monthly":
		return current.AddDate(0, 1, 0)
	case "yearly":
		return current.AddDate(1, 0, 0)
	// Add other cases like "bi-weekly" if needed:
	// case "bi-weekly":
	//	 return current.AddDate(0, 0, 14)
	default:
		slog.Warn("unsupported recurrence period encountered", "period", period)
		return time.Time{} // Return zero time for unsupported periods
	}
}

// --- Helper functions ---

func sqlNullStringToPointer(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

func determineSharingStatus(isShared bool, takesAll bool, partnerName *string) string {
	if !isShared {
		return "Alone"
	}
	partner := "Partner" // Default name
	if partnerName != nil && *partnerName != "" {
		partner = *partnerName
	}
	if takesAll {
		return "Paid by " + partner
	}
	return "Shared with " + partner
}
