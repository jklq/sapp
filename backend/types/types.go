package types

import "time"

// --- Shared Enums / Constants ---

// EditableSharingStatus defines the possible states the frontend can send for updates.
type EditableSharingStatus string

const (
	StatusAlone         EditableSharingStatus = "Alone"
	StatusShared        EditableSharingStatus = "Shared"
	StatusPaidByPartner EditableSharingStatus = "Paid by Partner"
)

// --- API Payloads and Responses ---

// PayPayload defines the structure for the manual payment request body.
type PayPayload struct {
	SharedStatus string  `json:"shared_status"` // 'alone' or 'shared'
	Amount       float64 `json:"amount"`
	Category     string  `json:"category"`              // Category name
	SpendingDate *string `json:"spending_date,omitempty"` // Optional: Date of spending "YYYY-MM-DD"
	PreSettled   bool    `json:"pre_settled"`           // New flag
}

// LoginRequest defines the structure for the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse defines the structure for the login response body
type LoginResponse struct {
	AccessToken  string `json:"access_token"` // Renamed from token
	RefreshToken string `json:"refresh_token"`
	UserID       int64  `json:"user_id"`
	FirstName    string `json:"first_name"`
}

// RefreshTokenRequest defines the structure for the token refresh request body
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshTokenResponse defines the structure for the token refresh response body
type RefreshTokenResponse struct {
	AccessToken string `json:"access_token"`
	// Optionally include a new refresh token if implementing rotation
	// RefreshToken string `json:"refresh_token,omitempty"`
}

// VerifyResponse defines the structure for the token verification response body
type VerifyResponse struct {
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
	// Add any other user details the frontend might need initially
}

// PartnerRegistrationRequest defines the structure for the partner registration request body
type PartnerRegistrationRequest struct {
	User1 UserRegistrationDetails `json:"user1"`
	User2 UserRegistrationDetails `json:"user2"`
}

// UserRegistrationDetails contains the details needed to register a single user
type UserRegistrationDetails struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
}

// PartnerRegistrationResponse defines the structure for the partner registration response body
type PartnerRegistrationResponse struct {
	Message string `json:"message"`
	User1ID int64  `json:"user1_id"`
	User2ID int64  `json:"user2_id"`
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

// UpdateSpendingPayload defines the structure for the update request body.
type UpdateSpendingPayload struct {
	Description   string                `json:"description"`
	CategoryName  string                `json:"category_name"`
	SharingStatus EditableSharingStatus `json:"sharing_status"`
}

// TransferStatusResponse defines the structure for the balance status.
type TransferStatusResponse struct {
	PartnerName string  `json:"partner_name"`
	AmountOwed  float64 `json:"amount_owed"` // Always positive, indicates the magnitude of the debt
	OwedBy      *string `json:"owed_by"`     // Name of the person who owes money (null if settled)
	OwedTo      *string `json:"owed_to"`     // Name of the person who is owed money (null if settled)
}

// AICategorizationPayload defines the structure for the AI categorization request body.
type AICategorizationPayload struct {
	Amount          float64 `json:"amount"`
	Prompt          string  `json:"prompt"`
	TransactionDate *string `json:"transaction_date,omitempty"` // Optional: Date of transaction "YYYY-MM-DD"
	PreSettled      bool    `json:"pre_settled"`                // Flag to mark as settled immediately
}

// --- Core Data Structures ---

// Category represents a category record in the database.
type Category struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	AINotes string `json:"ai_notes,omitempty"` // Include notes, omitempty if not needed in all contexts
}

// Deposit represents a deposit record (template) in the database.
type Deposit struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"user_id"` // Keep internal for now, response might just confirm success
	Amount           float64    `json:"amount"`
	Description      string     `json:"description"`
	DepositDate      time.Time  `json:"deposit_date"` // Start date for recurring, or date for one-off
	IsRecurring      bool       `json:"is_recurring"`
	RecurrencePeriod *string    `json:"recurrence_period"` // Pointer for nullable
	EndDate          *time.Time `json:"end_date"`          // Pointer for nullable end date of recurrence
	CreatedAt        time.Time  `json:"created_at"`
}

// --- Export Types ---

// UserExport defines the structure for exporting user details.
type UserExport struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

// CategoryExport defines the structure for exporting category details.
type CategoryExport struct {
	Name    string  `json:"name"`
	AINotes *string `json:"ai_notes,omitempty"` // Use pointer for optional field
}

// SpendingItemExport defines the structure for exporting individual spending items within a job or manual entry.
type SpendingItemExport struct {
	CategoryName  string  `json:"category_name"`
	Amount        float64 `json:"amount"`
	Description   string  `json:"description"`
	ApportionMode string  `json:"apportion_mode"` // "Alone", "Shared", "PaidByPartner"
}

// AIJobExport defines the structure for exporting AI categorization jobs and their spendings.
type AIJobExport struct {
	Prompt          string               `json:"prompt"`
	TotalAmount     float64              `json:"total_amount"`
	TransactionDate time.Time            `json:"transaction_date"`
	PreSettled      bool                 `json:"pre_settled"`
	BuyerUsername   string               `json:"buyer_username"` // Username of the person who submitted the job
	IsAmbiguous     bool                 `json:"is_ambiguous"`
	AmbiguityReason *string              `json:"ambiguity_reason,omitempty"`
	Spendings       []SpendingItemExport `json:"spendings"`
}

// ManualSpendingExport defines the structure for exporting manually added spendings.
type ManualSpendingExport struct {
	Amount        float64    `json:"amount"`
	Description   string     `json:"description"` // Description from spendings table
	CategoryName  string     `json:"category_name"`
	SpendingDate  time.Time  `json:"spending_date"`
	BuyerUsername string     `json:"buyer_username"` // Username of the person who paid
	SharedStatus  string     `json:"shared_status"`  // "Alone", "Shared", "PaidByPartner"
	SettledAt     *time.Time `json:"settled_at,omitempty"`
}

// DepositExport defines the structure for exporting deposit templates.
type DepositExport struct {
	Description      string     `json:"description"`
	Amount           float64    `json:"amount"`
	DepositDate      time.Time  `json:"deposit_date"` // Start date
	IsRecurring      bool       `json:"is_recurring"`
	RecurrencePeriod *string    `json:"recurrence_period,omitempty"`
	EndDate          *time.Time `json:"end_date,omitempty"`
	OwnerUsername    string     `json:"owner_username"` // Username of the deposit owner
}

// TransferExport defines the structure for exporting settlement records.
type TransferExport struct {
	SettlementTime    time.Time `json:"settlement_time"`
	SettledByUsername string    `json:"settled_by_username"` // Username of the user who initiated the settlement
}

// FullExport defines the overall structure for the exported data file.
type FullExport struct {
	ExportedAt      time.Time              `json:"exported_at"`
	User            UserExport             `json:"user"`
	Partner         UserExport             `json:"partner"`
	Categories      []CategoryExport       `json:"categories"`
	AIJobs          []AIJobExport          `json:"ai_jobs"`
	ManualSpendings []ManualSpendingExport `json:"manual_spendings"`
	Deposits        []DepositExport        `json:"deposits"`
	Transfers       []TransferExport       `json:"transfers"`
}

// --- End Export Types ---

// --- Stats Types ---

// CategorySpendingStat represents the total spending for a specific category.
type CategorySpendingStat struct {
	CategoryName string  `json:"category_name"`
	TotalAmount  float64 `json:"total_amount"`
}

// DepositStatsResponse defines the structure for the deposit statistics response.
type DepositStatsResponse struct {
	TotalAmount float64 `json:"total_amount"`
	Count       int     `json:"count"` // Number of deposits included in the sum
}

// --- End Stats Types ---

// UpdateDepositPayload defines the structure for the update deposit request body.
type UpdateDepositPayload struct {
	Amount           *float64 `json:"amount,omitempty"`           // Optional: only update if provided
	Description      *string  `json:"description,omitempty"`      // Optional
	DepositDate      *string  `json:"deposit_date,omitempty"`     // Optional: Format "YYYY-MM-DD"
	IsRecurring      *bool    `json:"is_recurring,omitempty"`     // Optional
	RecurrencePeriod *string  `json:"recurrence_period,omitempty"` // Optional: Can be nullified
	EndDate          *string  `json:"end_date,omitempty"`         // Optional: Format "YYYY-MM-DD" or null to clear
}

// UpdateDepositResponse defines the structure for the update deposit response body.
type UpdateDepositResponse struct {
	Message string `json:"message"`
	Deposit Deposit `json:"deposit"` // Return the updated deposit
}

// DeleteDepositResponse defines the structure for the delete deposit response body.
type DeleteDepositResponse struct {
	Message string `json:"message"`
}

// SpendingItem represents a single item within a transaction group (often generated by AI).
// Used in TransactionGroup and potentially other contexts.
type SpendingItem struct {
	ID                 int64     `json:"id"` // spendings.id
	Amount             float64   `json:"amount"`
	Description        string    `json:"description"`
	CategoryName       string    `json:"category_name"`
	SpendingDate       time.Time `json:"spending_date"`         // Actual date the spending occurred
	BuyerName          string    `json:"buyer_name"`            // Name of the user who paid for the original transaction
	PartnerName        *string   `json:"partner_name"`          // Name of the partner involved, if any
	SharedUserTakesAll bool      `json:"shared_user_takes_all"` // True if partner pays this item's full cost
	SharingStatus      string    `json:"sharing_status"`        // Derived: "Alone", "Shared with X", "Paid by X"
}

// TransactionGroup represents a single purchase/submission, potentially containing multiple spending items.
type TransactionGroup struct {
	// Type                string         `json:"type"` // Type identifier often added by handler/service
	JobID               int64          `json:"job_id"` // ai_categorization_jobs.id
	Prompt              string         `json:"prompt"`
	TotalAmount         float64        `json:"total_amount"`
	TransactionDate     time.Time      `json:"date"` // Job's transaction date (or creation if not set)
	BuyerName           string         `json:"buyer_name"`
	IsAmbiguityFlagged  bool           `json:"is_ambiguity_flagged"`
	AmbiguityFlagReason *string        `json:"ambiguity_flag_reason"` // Pointer to handle NULL/empty
	Spendings           []SpendingItem `json:"spendings"`
}

// DepositItem represents a single deposit occurrence (either original non-recurring or generated recurring).
// Used by history service and potentially API responses.
type DepositItem struct {
	// Type             string     `json:"type"` // Type identifier often added by handler/service
	ID               int64      `json:"id"`                // ID of the original deposit template
	Amount           float64    `json:"amount"`
	Description      string     `json:"description"`
	Date             time.Time  `json:"date"`              // The actual date of this occurrence
	IsRecurring      bool       `json:"is_recurring"`      // Indicates if this is a generated occurrence from a template
	RecurrencePeriod *string    `json:"recurrence_period"` // Period of the original template
	EndDate          *time.Time `json:"end_date"`          // End date of the original template (pointer for nullable)
	CreatedAt        time.Time  `json:"created_at"`        // Creation time of the original template
}
