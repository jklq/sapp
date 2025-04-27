// --- Shared Enums / Constants ---

// Represents the possible sharing states a user can select when editing
export type EditableSharingStatus = 'Alone' | 'Shared' | 'Paid by Partner';

// --- API Payloads and Responses ---

export function storeToken(token: string): void {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function getToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function removeToken(): void {
// Payload for manual payment submission
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
}

// Payload for the login request
export interface LoginPayload {
  username: string;
  password: string;
}

// Response from the login endpoint
export interface LoginResponse {
  token: string;
  user_id: number;
  first_name: string;
}

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

// Payload for adding a new deposit
export interface AddDepositPayload {
  amount: number;
  description: string;
  deposit_date: string; // Format: "YYYY-MM-DD"
  is_recurring: boolean;
  recurrence_period?: string | null; // Optional, required if is_recurring is true
}

// Response from the add deposit endpoint (example, adjust if needed)
export interface AddDepositResponse {
    message: string;
    deposit_id: number;
}

// Payload for updating a spending item
export interface UpdateSpendingPayload {
  description: string;
  category_name: string;
  sharing_status: EditableSharingStatus;
}

// Response from the GET /v1/transfer/status endpoint
export interface TransferStatusResponse {
  partner_name: string;
  amount_owed: number; // Always positive, indicates the magnitude of the debt
  owed_by: string | null; // Name of the person who owes (null if settled)
  owed_to: string | null; // Name of the person who is owed (null if settled)
}


// --- Core Data Structures ---

export interface Category {
  id: number;
  name: string;
  // ai_notes is likely backend-only
}

// Represents a single spending item within a transaction group
// This structure is used within the HistoryListItem for spending groups
export interface SpendingItem {
  id: number;
  amount: number;
  description: string;
  category_name: string;
  buyer_name: string;
  partner_name: string | null;
  shared_user_takes_all: boolean;
  sharing_status: string; // Derived: "Alone", "Shared with X", "Paid by X"
}

// Represents a generic history item returned by the /v1/history endpoint
// Fields are optional because an item is either a spending group OR a deposit
export interface HistoryListItem {
  // Common fields
  type: 'spending_group' | 'deposit';
  date: string; // ISO date string for sorting (job creation or deposit occurrence)

  // Fields from TransactionGroup (present if type is 'spending_group')
  job_id?: number;
  prompt?: string;
  total_amount?: number;
  buyer_name?: string;
  is_ambiguity_flagged?: boolean;
  ambiguity_flag_reason?: string | null;
  spendings?: SpendingItem[];

  // Fields from DepositItem (present if type is 'deposit')
  id?: number; // Deposit ID (original template ID)
  amount?: number;
  description?: string;
  is_recurring?: boolean;
  recurrence_period?: string | null;
  created_at?: string; // Deposit template creation time
}

// Response from the GET /v1/history endpoint
export interface HistoryResponse {
  history: HistoryListItem[]; // A flat, sorted list of items
}


// --- Deprecated / Old Types (Can be removed if no longer used) ---

// Structure for detailed spending info fetched from the backend (Likely replaced by HistoryListItem)
// export interface SpendingDetail {
//   id: number;
//   amount: number;
//   description: string;
//   category_name: string;
//   created_at: string; // ISO date string
//   buyer_name: string;
//   partner_name: string | null; // Can be null if not shared or partner name missing
//   shared_user_takes_all: boolean;
//   sharing_status: string; // Derived status: "Alone", "Shared with X", "Paid by X"
// }

// Represents a group of spendings originating from one AI job/transaction (Replaced by HistoryListItem type='spending_group')
// export interface TransactionGroup {
//   job_id: number;
//   prompt: string;
//   total_amount: number;
//   job_created_at: string; // ISO date string for the job creation
//   buyer_name: string; // Added: Name of the user who submitted the job
//   is_ambiguity_flagged: boolean;
//   ambiguity_flag_reason: string | null;
//   spendings: SpendingItem[]; // The list of individual spendings for this job
// }

// Type for the response from the updated /v1/spendings endpoint (Replaced by HistoryResponse)
// export type GroupedSpendingsResponse = TransactionGroup[];


// Represents a deposit item fetched from the backend history endpoint (Replaced by HistoryListItem type='deposit')
// export interface DepositItem {
//   id: number;
//   type: 'deposit'; // Identifier
//   amount: number;
//   description: string;
//   date: string; // ISO date string
//   is_recurring: boolean;
//   recurrence_period: string | null;
//   created_at: string; // ISO date string
// }
  const token = getToken();
  const headers = new Headers(options.headers || {});

  if (token) {
    // Prepend "Bearer " to the token for standard JWT authorization
    headers.set('Authorization', `Bearer ${token}`);
  }
  // Ensure Content-Type is set for methods that have a body
  if (options.body && !headers.has('Content-Type')) {
      if (typeof options.body === 'string') {
          // Assume JSON if it's a stringified object, otherwise default or let browser handle
          try {
              JSON.parse(options.body);
              headers.set('Content-Type', 'application/json');
          } catch (e) {
              // Not JSON, maybe form data or plain text - don't set default
          }
      }
      // If body is FormData, browser sets Content-Type automatically (multipart/form-data)
      // If body is URLSearchParams, browser sets Content-Type automatically (application/x-www-form-urlencoded)
  }


  return fetch(url, {
    ...options,
    headers: headers,
  });
}


// --- API Functions ---

export async function fetchCategories(): Promise<Category[]> {
  const response = await fetchWithAuth(`${API_BASE_URL}/v1/categories`);
  if (!response.ok) {
    throw new Error(`Failed to fetch categories: ${response.statusText}`);
  }
  return await response.json();
}

// Refactored to send JSON body instead of using path parameters
export async function submitManualPayment(payload: PayPayload): Promise<void> {
  const url = `${API_BASE_URL}/v1/pay`; // Use base path

  const response = await fetchWithAuth(url, {
    method: "POST",
    body: JSON.stringify(payload), // Send payload as JSON body
    // fetchWithAuth will set Content-Type: application/json
  });

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(
      `Failed to submit payment: ${response.statusText} - ${errorBody}`
    );
  }

  // Check if the response status is 201 Created
  if (response.status !== 201) {
    const responseBody = await response.text();
    console.warn(`Unexpected status code: ${response.status}`, responseBody);
    // Optionally throw an error or handle differently
  }

  // No content expected on success (201 Created) based on backend code
}


export async function submitAICategorization(payload: AICategorizationPayload): Promise<void> {
  const url = `${API_BASE_URL}/v1/categorize`;

  const response = await fetchWithAuth(url, {
    method: "POST",
    body: JSON.stringify(payload), // Include pre_settled flag if present
    // fetchWithAuth will set Content-Type: application/json
  });

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(
      `Failed to submit for AI categorization: ${response.statusText} - ${errorBody}`
    );
  }

   // Check if the response status indicates success (202 Accepted for async job submission)
   if (response.status !== 202) {
    const responseBody = await response.text();
    console.warn(`Unexpected status code from AI categorization endpoint: ${response.status}`, responseBody);
    // Optionally throw an error or handle differently based on other success codes if needed
    throw new Error(
        `AI categorization submission failed with status: ${response.status} - ${responseBody}`
    );
  }

  // Handle response if needed (e.g., getting a job ID back)
  // The backend currently returns the job_id in the body on 202
  // You might want to parse and return this if the frontend needs it
  // const data = await response.json();
  // return data.job_id; // Example
}


// --- Auth API Functions ---

// New function: Logs in the user
export async function loginUser(payload: LoginPayload): Promise<LoginResponse> {
  const url = `${API_BASE_URL}/v1/login`;

  const response = await fetch(url, { // Login doesn't need auth token initially
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(payload),
  });

  if (!response.ok) {
    // Try to parse error response from backend if possible
    let errorBody = `Login failed: ${response.statusText}`;
    try {
        const errData = await response.json();
        errorBody = errData.message || errData.error || errorBody; // Adjust based on backend error format
    } catch (e) {
        // Ignore if response is not JSON
    }
    throw new Error(errorBody);
  }

  // Assuming the backend returns JSON with token, user_id, first_name
  const data: LoginResponse = await response.json();
  if (!data.token) {
      throw new Error("Login successful, but no token received.");
  }
  return data;
}

// New function: Registers two users as partners
export async function registerPartners(payload: PartnerRegistrationPayload): Promise<PartnerRegistrationResponse> {
    const url = `${API_BASE_URL}/v1/register/partners`;

    const response = await fetch(url, { // Registration is public, no auth token needed
        method: "POST",
        headers: {
            "Content-Type": "application/json",
        },
        body: JSON.stringify(payload),
    });

    if (!response.ok) {
        // Try to parse error response from backend if possible
        let errorBody = `Registration failed: ${response.statusText}`;
        try {
            const errData = await response.json();
            errorBody = errData.message || errData.error || errorBody; // Adjust based on backend error format
        } catch (e) {
             try {
                // If not JSON, try reading as text
                const textError = await response.text();
                if (textError) {
                    errorBody += ` - ${textError}`;
                }
            } catch (textErr) {
                // Ignore if reading text also fails
            }
        }
        throw new Error(errorBody);
    }

    // Expecting 201 Created with JSON body on success
    if (response.status !== 201) {
         console.warn(`Unexpected status code after partner registration: ${response.status}`);
         // Optionally throw error if status is not 201
    }

    const data: PartnerRegistrationResponse = await response.json();
    return data;
}


// --- Deposit API Functions ---

// Adds a new deposit record
export async function addDeposit(payload: AddDepositPayload): Promise<{ message: string; deposit_id: number }> {
    const url = `${API_BASE_URL}/v1/deposits`;
    const response = await fetchWithAuth(url, {
        method: "POST",
        body: JSON.stringify(payload),
        // fetchWithAuth will set Content-Type: application/json
    });

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to add deposit: ${response.statusText}`;
        try {
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch (e) {
            errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    // Expecting 201 Created with JSON body on success
    if (response.status !== 201) {
        console.warn(`Unexpected status code after adding deposit: ${response.status}`);
    }

    const data: { message: string; deposit_id: number } = await response.json();
    return data;
}


// --- History API Functions ---

// Fetches combined history (spending groups and deposits)
export async function fetchHistory(): Promise<HistoryResponse> {
    const url = `${API_BASE_URL}/v1/history`; // Updated endpoint
    const response = await fetchWithAuth(url); // GET request by default

    if (!response.ok) {
        const errorBody = await response.text();
        throw new Error(`Failed to fetch history: ${response.statusText} - ${errorBody}`);
    }

    const data: HistoryResponse = await response.json();
    console.log("Fetched History:", data); // Debug log
    return data;
}

// New function: Updates a specific spending item
export async function updateSpendingItem(spendingId: number, payload: UpdateSpendingPayload): Promise<void> {
    const url = `${API_BASE_URL}/v1/spendings/${spendingId}`;

    const response = await fetchWithAuth(url, {
        method: "PUT",
        body: JSON.stringify(payload),
        // fetchWithAuth will set Content-Type: application/json
    });

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to update spending item: ${response.statusText}`;
        try {
            // Try to parse backend error message
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch (e) {
            // Use text if not JSON
            errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    // Expecting 200 OK with no body on success
    if (response.status !== 200) {
        console.warn(`Unexpected status code after updating spending item: ${response.status}`);
        // Optionally handle other success codes if the backend changes
    }
}

// New function: Deletes an AI job and its associated spendings
export async function deleteAIJob(jobId: number): Promise<void> {
    const url = `${API_BASE_URL}/v1/jobs/${jobId}`;

    const response = await fetchWithAuth(url, {
        method: "DELETE",
    });

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to delete job: ${response.statusText}`;
        try {
            // Try to parse backend error message
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch (e) {
            // Use text if not JSON
            errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    // Expecting 204 No Content on success
    if (response.status !== 204) {
        console.warn(`Unexpected status code after deleting job: ${response.status}`);
        // Optionally handle other success codes if the backend changes
    }
}


// --- Transfer API Functions ---

// Fetches the current transfer status between the user and partner
export async function fetchTransferStatus(): Promise<TransferStatusResponse> {
    const url = `${API_BASE_URL}/v1/transfer/status`;
    const response = await fetchWithAuth(url); // GET request

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to fetch transfer status: ${response.statusText}`;
        try {
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch (e) {
             errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    const data: TransferStatusResponse = await response.json();
    console.log("Fetched Transfer Status:", data); // Debug log
    return data;
}

// Records that a transfer/settlement has occurred
export async function recordTransfer(): Promise<void> {
    const url = `${API_BASE_URL}/v1/transfer/record`;
    const response = await fetchWithAuth(url, { method: "POST" });

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to record transfer: ${response.statusText}`;
         try {
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch (e) {
             errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    // Expecting 200 OK on success
    if (response.status !== 200) {
        console.warn(`Unexpected status code after recording transfer: ${response.status}`);
    }
}
