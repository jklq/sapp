export interface Category {
  id: number;
  name: string;
}

// Payload for manual payment submission
export interface PayPayload {
  shared_status: "alone" | "shared"; // Backend currently only supports these
  amount: number;
  category: string;
  pre_settled?: boolean; // Optional: Flag to mark as settled immediately
}

// Payload specifically for triggering AI categorization
type AICategorizationPayload struct {
	Amount       float64 `json:"amount"`
	Prompt       string  `json:"prompt"`
	SpendingDate *string `json:"spending_date,omitempty"` // Optional: Date of spending "YYYY-MM-DD"
	PreSettled   bool    `json:"pre_settled"`           // Optional: Flag to mark as settled immediately
	// Note: category is NOT included here as the AI determines it.
}

// Payload for the login request
export interface LoginPayload {
  username: string;
  password: string;
}

// Response from the login endpoint
export interface LoginResponse {
  token: string;
  user_id: number; // Assuming backend sends user_id
  first_name: string; // Assuming backend sends first_name
}

// Structure for detailed spending info fetched from the backend
export interface SpendingDetail {
  id: number;
  amount: number;
  description: string;
  category_name: string;
  created_at: string;
  buyer_name: string;
  partner_name: string | null; // Can be null if not shared or partner name missing
  shared_user_takes_all: boolean;
  sharing_status: string; // Derived status: "Alone", "Shared with X", "Paid by X"
}

// --- Types for Grouped Spending View ---

// Represents a single spending item within a transaction group
export interface SpendingItem {
  id: number;
  amount: number;
  description: string;
  category_name: string;
  buyer_name: string; // Person who paid for the original transaction
  partner_name: string | null;
  shared_user_takes_all: boolean;
  sharing_status: string; // Derived: "Alone", "Shared with X", "Paid by X"
}

// Represents a group of spendings originating from one AI job/transaction
export interface TransactionGroup {
  job_id: number;
  prompt: string;
  total_amount: number;
  job_created_at: string;
  buyer_name: string;
  is_ambiguity_flagged: boolean;
  ambiguity_flag_reason: string | null;
  spendings: SpendingItem[];
}

// Type for the response from the updated /v1/spendings endpoint
export type GroupedSpendingsResponse = TransactionGroup[];

// --- Types for Editing Spendings ---

// Represents the possible sharing states a user can select when editing
export type EditableSharingStatus = "Alone" | "Shared" | "Paid by Partner";

// Payload for updating a spending item (excluding amount for now)
export interface UpdateSpendingPayload {
  description: string;
  category_name: string;
  sharing_status: EditableSharingStatus;
}

// --- Types for Transfer Page ---

// Response from the GET /v1/transfer/status endpoint
export interface TransferStatusResponse {
  partner_name: string;
  amount_owed: number; // Always positive, indicates the magnitude of the debt
  owed_by: string | null; // Name of the person who owes (null if settled)
  owed_to: string | null; // Name of the person who is owed (null if settled)
}

// --- Types for Partner Registration ---

// Details for registering a single user within the partner registration form
export interface UserRegistrationDetails {
  username: string;
  password: string;
  first_name: string;
}

// Payload for the POST /v1/register/partners endpoint
export interface PartnerRegistrationPayload {
  user1: UserRegistrationDetails;
  user2: UserRegistrationDetails;
}

// Response from the POST /v1/register/partners endpoint
export interface PartnerRegistrationResponse {
  message: string;
  user1_id: number;
  user2_id: number;
}

// --- Types for Deposits ---

// Represents a deposit item (occurrence) fetched from the backend history endpoint
// Note: This represents an *occurrence*, not the template itself.
// It includes the original template ID.
export interface DepositItem {
  id: number; // ID of the original template
  type: "deposit";
  amount: number;
  description: string;
  date: string;
  is_recurring: boolean; // True if generated from a recurring template
  recurrence_period: string | null; // Period of the template
  end_date?: string | null; // Optional: End date of the template (ISO date string or null)
  created_at: string;
}

// Payload for adding a new deposit
export interface AddDepositPayload {
  amount: number;
  description: string;
  deposit_date: string; // Format: "YYYY-MM-DD"
  is_recurring: boolean;
  recurrence_period?: string | null; // Optional, required if is_recurring is true
}

// Response from adding a new deposit
export interface AddDepositResponse {
  message: string;
  deposit_id: number;
}

// Payload for updating an existing deposit template
export interface UpdateDepositPayload {
  amount?: number;
  description?: string;
  deposit_date?: string; // Format: "YYYY-MM-DD"
  is_recurring?: boolean;
  recurrence_period?: string | null; // Can be nullified
  end_date?: string | null; // Format: "YYYY-MM-DD" or null to clear
}

// Response from deleting a deposit
export interface DeleteDepositResponse {
  message: string;
}

// Represents the full deposit template details (used for editing)
export interface DepositTemplate extends DepositItem {
  // Inherits fields from DepositItem (representing the template itself)
}

// --- Type for Combined History ---

// Represents a generic history item from the backend
// The actual data is nested within based on the 'type' field
export interface HistoryListItem {
  type: "spending_group" | "deposit";
  date: string; // ISO date string for sorting (job creation or deposit occurrence)
  // The rest of the fields depend on the 'type'
  // We use 'any' here for simplicity, but discriminated unions are better if feasible
  // Or the component can cast based on 'type'
  [key: string]: any; // Allow other properties
}

// Response from the GET /v1/history endpoint
export interface HistoryResponse {
  history: HistoryListItem[]; // A flat, sorted list of items
}
