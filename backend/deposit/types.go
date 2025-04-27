package deposit

import "time"

// Deposit represents a deposit record in the database.
type Deposit struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"` // Keep internal for now, response might just confirm success
	Amount           float64    `json:"amount"`
	Description      string     `json:"description"`
	DepositDate      time.Time  `json:"deposit_date"`
	IsRecurring      bool       `json:"is_recurring"`
	RecurrencePeriod *string    `json:"recurrence_period"` // Pointer for nullable
	CreatedAt        time.Time  `json:"created_at"`
}

// AddDepositPayload defines the structure for the add deposit request body.
type AddDepositPayload struct {
	Amount           float64 `json:"amount"`
	Description      string  `json:"description"`
	DepositDate      string  `json:"deposit_date"` // Expecting ISO 8601 date string e.g., "YYYY-MM-DD"
	IsRecurring      bool    `json:"is_recurring"`
	RecurrencePeriod *string `json:"recurrence_period,omitempty"` // Optional
}

// AddDepositResponse defines the structure for the add deposit response body.
type AddDepositResponse struct {
	Message   string `json:"message"`
	DepositID int64  `json:"deposit_id"`
}
