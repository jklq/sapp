export interface Category {
  id: number;
  name: string;
}

export interface PayPayload {
  shared_status: 'alone' | 'shared' | 'mix'; // Assuming 'mix' might be added later based on backend code
  amount: number;
  category: string; // Category name
  // The backend category worker uses a 'prompt', let's add a field for that.
  // The current pay endpoint doesn't take it, but the categorization likely needs it.
  // We might need to adjust the backend pay endpoint later or add a new one.
  // For now, let's collect it in the UI.
  prompt: string;
}

// Payload specifically for triggering AI categorization
// shared_status is removed, AI will infer this from the prompt
export interface AICategorizationPayload {
  amount: number;
  prompt: string;
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
