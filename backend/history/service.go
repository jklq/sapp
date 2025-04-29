package history

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/types"
)

// HistoryListItem represents a generic item in the combined history list for internal processing.
// It includes common fields for sorting and identification, and the raw item.
type HistoryListItem struct {
	Type    string      `json:"type"` // "spending_group" or "deposit"
	Date    time.Time   `json:"date"` // Primary sorting key (job creation time or deposit occurrence date)
	RawItem interface{} `json:"-"`    // Store the original struct (types.TransactionGroup or types.DepositItem), ignored by JSON

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
			Date:    g.TransactionDate, // Use job's transaction date for sorting
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

// fetchSpendingGroups fetches transaction groups (as types.TransactionGroup) initiated by the user OR their partner.
func fetchSpendingGroups(db *sql.DB, userID int64) ([]types.TransactionGroup, error) {
	// Get partner ID first
	partnerID, partnerOk := auth.GetPartnerUserID(db, userID)
	if !partnerOk {
		// If no partner, proceed as before, only fetching user's jobs
		partnerID = -1 // Use an invalid ID to ensure partner clause doesn't match
		slog.Debug("No partner found, fetching only user's spending groups", "user_id", userID)
	}

	groups := []types.TransactionGroup{} // Use types.TransactionGroup

	// Query Explanation:
	// Selects jobs where:
	// 1. The buyer is the requesting user (j.buyer = userID)
	// OR
	// 2. The buyer is the partner (j.buyer = partnerID) AND there exists at least one spending item
	//    in that job (linked via ai_categorized_spendings) where the requesting user is the
	//    'shared_with' participant (us.shared_with = userID).
	// This ensures we only get partner's jobs if the requesting user is actually involved.
	jobQuery := `
		SELECT DISTINCT
			j.id, j.prompt, j.total_amount, j.transaction_date AS date, j.is_ambiguity_flagged, j.ambiguity_flag_reason, u.first_name AS buyer_name, j.buyer
		FROM ai_categorization_jobs j
		JOIN users u ON j.buyer = u.id
		LEFT JOIN ai_categorized_spendings acs ON j.id = acs.job_id
		LEFT JOIN user_spendings us ON acs.spending_id = us.spending_id
		WHERE j.buyer = ? OR (j.buyer = ? AND us.shared_with = ?)
		ORDER BY j.transaction_date DESC, j.created_at DESC; -- Sort primarily by transaction date
	`
	// Note: Using LEFT JOINs and DISTINCT because a job might have multiple spendings involving the user.
	// We only need to know *if* the user is involved in *any* spending for partner-bought jobs.

	jobRows, err := db.Query(jobQuery, userID, partnerID, userID) // Pass userID, partnerID, userID
	if err != nil {
		slog.Error("failed to query AI categorization jobs for user and involved partner jobs", "user_id", userID, "partner_id", partnerID, "err", err)
		return nil, err
	}
	defer jobRows.Close()

	spendingQuery := `
		SELECT
			s.id, s.amount, s.description, c.name AS category_name,
			u_buyer.first_name AS buyer_name, u_partner.first_name AS partner_name,
			us.shared_user_takes_all, us.shared_with, us.buyer -- Select buyer ID from user_spendings
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

	// Fetch requesting user's name once for determineSharingStatus
	var requestingUserName string
	err = db.QueryRow("SELECT first_name FROM users WHERE id = ?", userID).Scan(&requestingUserName)
	if err != nil {
		slog.Error("failed to fetch requesting user's name for history", "user_id", userID, "err", err)
		// Proceed without name, status strings might be less specific
		requestingUserName = "You"
	}

	for jobRows.Next() {
		var group types.TransactionGroup // Use types.TransactionGroup
		var ambiguityReason sql.NullString
		var jobBuyerID int64 // To store the buyer ID from the job

		if err := jobRows.Scan(
			&group.JobID, &group.Prompt, &group.TotalAmount, &group.TransactionDate, // Scan directly into TransactionDate field
			&group.IsAmbiguityFlagged, &ambiguityReason, &group.BuyerName, &jobBuyerID, // Scan jobBuyerID
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
			var itemBuyerID int64 // To store the buyer ID from user_spendings

			if err := spendingRows.Scan(
				&item.ID, &item.Amount, &item.Description, &item.CategoryName,
				&item.BuyerName, &partnerName, &item.SharedUserTakesAll, &sharedWithID, &itemBuyerID, // Scan itemBuyerID
			); err != nil {
				slog.Error("failed to scan spending item row for history", "user_id", userID, "job_id", group.JobID, "err", err)
				spendingRows.Close()
				return nil, err
			}

			item.PartnerName = sqlNullStringToPointer(partnerName)
			// Determine status from the perspective of the requesting user (userID)
			// Use itemBuyerID (from user_spendings) and sharedWithID for accurate status
			item.SharingStatus = determineSharingStatus(userID, itemBuyerID, sharedWithID.Valid, item.SharedUserTakesAll, item.PartnerName, requestingUserName)
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
	// Fetch deposit templates including the end_date
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
		var d types.DepositItem  // Use types.DepositItem
		var endDate sql.NullTime // Need to scan nullable end_date

		if err := depositRows.Scan(
			&d.ID,
			&d.Amount,
			&d.Description,
			&d.Date, // Scan directly into d.Date
			&d.IsRecurring,
			&d.RecurrencePeriod, // Scan directly into pointer field (driver handles null)
			&endDate,            // Scan into sql.NullTime
			&d.CreatedAt,
		); err != nil {
			slog.Error("failed to scan deposit template row for history service", "user_id", userID, "err", err)
			return nil, err
		}
		// Convert sql.NullTime to *time.Time
		if endDate.Valid {
			d.EndDate = &endDate.Time
		}
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

// createOccurrence creates a types.DepositItem instance for a specific occurrence date,
// including the EndDate from the template.
func createOccurrence(template types.DepositItem, occurrenceDate time.Time) types.DepositItem {
	return types.DepositItem{
		ID:               template.ID, // Link back to the original template ID
		Amount:           template.Amount,
		Description:      template.Description,
		Date:             occurrenceDate,       // This specific occurrence's date
		IsRecurring:      template.IsRecurring, // Keep original template flag
		RecurrencePeriod: template.RecurrencePeriod,
		EndDate:          template.EndDate,   // Keep original template end date
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

// determineSharingStatus calculates the sharing status string from the perspective of the requesting user.
func determineSharingStatus(requestingUserID, buyerID int64, isShared bool, takesAll bool, partnerName *string, requestingUserName string) string {
	partner := "Partner" // Default partner name
	if partnerName != nil && *partnerName != "" {
		partner = *partnerName
	}

	if buyerID == requestingUserID {
		// Requesting user paid
		if !isShared {
			return "Alone"
		}
		if takesAll {
			// User paid, partner takes all -> Partner owes user full amount
			return fmt.Sprintf("Paid by %s", partner) // Corrected: Partner benefits, owes user
		}
		// User paid, shared 50/50
		return fmt.Sprintf("Shared with %s", partner)
	} else {
		// Partner (or someone else) paid
		if !isShared {
			// Partner paid, alone (doesn't involve requesting user) - Should ideally not be fetched, but handle defensively
			return fmt.Sprintf("%s Alone", partner) // Indicate partner paid alone
		}
		// Partner paid, shared with requesting user
		if takesAll {
			// Partner paid, user takes all -> User owes partner full amount
			return fmt.Sprintf("Paid by You (%s)", requestingUserName) // Corrected: User benefits, owes partner
		}
		// Partner paid, shared 50/50
		return fmt.Sprintf("Shared with You (%s)", requestingUserName)
	}
}
