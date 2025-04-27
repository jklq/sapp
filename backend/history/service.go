package history

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"git.sr.ht/~relay/sapp-backend/spendings" // Import spendings types (TransactionGroup, SpendingItem)
	// Note: We define DepositItem locally to avoid circular dependency with deposit package if needed,
	// or we ensure deposit types are suitable. Let's define it locally for clarity.
)

// HistoryListItem represents a generic item in the combined history list.
// It includes common fields for sorting and identification.
type HistoryListItem struct {
	Type     string    `json:"type"` // "spending_group" or "deposit"
	Date     time.Time `json:"date"` // Primary sorting key (job creation time or deposit occurrence date)
	RawItem  interface{} `json:"-"`    // Store the original struct (TransactionGroup or DepositItem), ignored by JSON
	// Add other common fields if necessary, e.g., ID, but types differ
}

// DepositItem represents a single deposit occurrence (either original non-recurring or generated recurring).
// This is similar to spendings.DepositItem but defined here for the service.
type DepositItem struct {
	ID               int64      `json:"id"`                // ID of the original deposit template for recurring items
	Type             string     `json:"type"`              // Always "deposit"
	Amount           float64    `json:"amount"`
	Description      string     `json:"description"`
	Date             time.Time  `json:"date"`              // The actual date of this occurrence
	IsRecurring      bool       `json:"is_recurring"`      // Indicates if this is a generated occurrence from a template
	RecurrencePeriod *string    `json:"recurrence_period"` // Period of the original template
	CreatedAt        time.Time  `json:"created_at"`        // Creation time of the original template
}

// GenerateHistory fetches and combines spending groups and deposit occurrences for a user within a given time range.
func GenerateHistory(db *sql.DB, userID int64, endDate time.Time) ([]HistoryListItem, error) {
	// We'll fetch all relevant items and generate occurrences up to the endDate.
	// For simplicity, we won't implement a startDate filter yet, but it could be added.

	allHistoryItems := []HistoryListItem{}
	now := time.Now().UTC() // Use consistent time for generation limit

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
	generatedDeposits := []DepositItem{}
	for _, deposit := range deposits {
		if deposit.IsRecurring && deposit.RecurrencePeriod != nil {
			// Generate occurrences for this recurring deposit
			occurrences := generateDepositOccurrences(deposit, now) // Generate up to 'now'
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

// fetchSpendingGroups fetches transaction groups initiated by the user.
// (This logic is extracted from the original HandleGetHistory)
func fetchSpendingGroups(db *sql.DB, userID int64) ([]spendings.TransactionGroup, error) {
	groups := []spendings.TransactionGroup{}
	jobQuery := `
		SELECT
			j.id, j.prompt, j.total_amount, j.created_at, j.is_ambiguity_flagged, j.ambiguity_flag_reason, u.first_name AS buyer_name
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
		var group spendings.TransactionGroup
		group.Type = "spending_group"
		var ambiguityReason sql.NullString

		if err := jobRows.Scan(
			&group.JobID, &group.Prompt, &group.TotalAmount, &group.Date,
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

		group.Spendings = []spendings.SpendingItem{}
		for spendingRows.Next() {
			var item spendings.SpendingItem
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

// fetchDeposits fetches all deposit records (templates and non-recurring) for the user.
func fetchDeposits(db *sql.DB, userID int64) ([]DepositItem, error) {
	fetchedDeposits := []DepositItem{}
	depositQuery := `
		SELECT id, amount, description, deposit_date, is_recurring, recurrence_period, created_at
		FROM deposits
		WHERE user_id = ?
		ORDER BY deposit_date DESC, created_at DESC;
	`
	depositRows, err := db.Query(depositQuery, userID)
	if err != nil {
		slog.Error("failed to query deposits for history service", "user_id", userID, "err", err)
		return nil, err
	}
	defer depositRows.Close()

	for depositRows.Next() {
		var d DepositItem
		d.Type = "deposit"
		var depositDateDB time.Time // Scan directly into time.Time

		if err := depositRows.Scan(
			&d.ID,
			&d.Amount,
			&d.Description,
			&depositDateDB, // Scan directly
			&d.IsRecurring,
			&d.RecurrencePeriod,
			&d.CreatedAt,
		); err != nil {
			slog.Error("failed to scan deposit row for history service", "user_id", userID, "err", err)
			return nil, err
		}
		d.Date = depositDateDB // Assign scanned date
		fetchedDeposits = append(fetchedDeposits, d)
	}
	if err := depositRows.Err(); err != nil {
		slog.Error("error iterating deposit rows for history service", "user_id", userID, "err", err)
		return nil, err
	}
	return fetchedDeposits, nil
}

// generateDepositOccurrences calculates future dates for a recurring deposit.
func generateDepositOccurrences(template DepositItem, endDate time.Time) []DepositItem {
	occurrences := []DepositItem{}
	currentDate := template.Date // Start from the initial deposit date

	// Ensure we don't generate occurrences before the template start date
	// and include the initial date itself if it's within range (it always should be based on fetch logic)
	if currentDate.After(endDate) {
		return occurrences // Template starts after the end date
	}

	// Add the initial occurrence
	occurrences = append(occurrences, createOccurrence(template, currentDate))

	// Generate subsequent occurrences
	for {
		nextDate := calculateNextDate(currentDate, *template.RecurrencePeriod)
		if nextDate.IsZero() || nextDate.After(endDate) {
			break // Stop if next date is invalid or past the end date
		}
		occurrences = append(occurrences, createOccurrence(template, nextDate))
		currentDate = nextDate
	}

	return occurrences
}

// createOccurrence creates a DepositItem instance for a specific occurrence date.
func createOccurrence(template DepositItem, occurrenceDate time.Time) DepositItem {
	return DepositItem{
		ID:               template.ID, // Link back to the original template ID
		Type:             "deposit",
		Amount:           template.Amount,
		Description:      template.Description,
		Date:             occurrenceDate, // This specific occurrence's date
		IsRecurring:      true,           // Mark as generated from a recurring template
		RecurrencePeriod: template.RecurrencePeriod,
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
