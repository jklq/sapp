export interface Category {
  id: number;
  name: string;
}

// Payload for manual payment submission (now uses JSON body)
export interface PayPayload {
  shared_status: 'alone' | 'shared'; // Backend currently only supports these
  amount: number;
  category: string; // Category name
  pre_settled?: boolean; // Optional: Flag to mark as settled immediately
}

// Payload specifically for triggering AI categorization
export interface AICategorizationPayload {
  amount: number;
  prompt: string;
  pre_settled?: boolean; // Optional: Flag to mark as settled immediately
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
  created_at: string; // ISO date string
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
  job_created_at: string; // ISO date string for the job creation
  buyer_name: string; // Added: Name of the user who submitted the job
  is_ambiguity_flagged: boolean;
  ambiguity_flag_reason: string | null;
  spendings: SpendingItem[]; // The list of individual spendings for this job
}

// Type for the response from the updated /v1/spendings endpoint
export type GroupedSpendingsResponse = TransactionGroup[];

// --- Types for Editing Spendings ---

// Represents the possible sharing states a user can select when editing
export type EditableSharingStatus = 'Alone' | 'Shared' | 'Paid by Partner';

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

// --- Types for Partner Registration --- are defined above now ---

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

// Represents a deposit item fetched from the backend history endpoint
export interface DepositItem {
  id: number;
  type: 'deposit'; // Identifier
  amount: number;
  description: string;
  date: string; // ISO date string
  is_recurring: boolean;
  recurrence_period: string | null;
  created_at: string; // ISO date string
}

// Payload for adding a new deposit
export interface AddDepositPayload {
  amount: number;
  description: string;
  deposit_date: string; // Format: "YYYY-MM-DD"
  is_recurring: boolean;
  recurrence_period?: string | null; // Optional, required if is_recurring is true
}

// --- Type for Combined History ---

// --- Type for Combined History ---

// Represents a generic history item from the backend
// The actual data is nested within based on the 'type' field
export interface HistoryListItem {
  type: 'spending_group' | 'deposit';
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
