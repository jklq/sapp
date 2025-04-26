import { Category, PayPayload, AICategorizationPayload, LoginPayload, LoginResponse, GroupedSpendingsResponse, UpdateSpendingPayload, TransferStatusResponse, PartnerRegistrationPayload, PartnerRegistrationResponse } from "./types";

// --- Token Management ---

const AUTH_TOKEN_KEY = 'authToken';

export function storeToken(token: string): void {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function getToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function removeToken(): void {
  localStorage.removeItem(AUTH_TOKEN_KEY);
}

// --- API Base URL ---
const API_BASE_URL = import.meta.env.VITE_API_URL || "http://localhost:3000";

// --- Helper for authenticated requests ---
async function fetchWithAuth(url: string, options: RequestInit = {}): Promise<Response> {
  const token = getToken();
  const headers = new Headers(options.headers || {});

  if (token) {
    // --- INSECURE: Sending UserID as raw token for Dev ---
    // TODO: Replace with proper Bearer token when JWT is implemented
    headers.set('Authorization', token); // Send raw token (user ID string)
    // --- End INSECURE ---
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

export async function submitManualPayment(payload: PayPayload): Promise<void> {
  const { shared_status, amount, category } = payload;
  if (!category) {
    throw new Error("Category is required for manual payment submission.");
  }
  const url = `${API_BASE_URL}/v1/pay/${shared_status}/${amount}/${encodeURIComponent(category)}`;

  const response = await fetchWithAuth(url, { method: "POST" });

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


// shared_status is removed from the payload
export async function submitAICategorization(payload: AICategorizationPayload): Promise<void> {
  const url = `${API_BASE_URL}/v1/categorize`;

  const response = await fetchWithAuth(url, {
    method: "POST",
    // fetchWithAuth will set Content-Type: application/json if needed
    body: JSON.stringify(payload),
    // fetchWithAuth will set Content-Type: application/json if needed
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


// --- Spendings API Functions ---

// Updated function: Fetches spendings grouped by transaction/AI job
export async function fetchSpendings(): Promise<GroupedSpendingsResponse> {
    const url = `${API_BASE_URL}/v1/spendings`;
    const response = await fetchWithAuth(url); // GET request by default

    if (!response.ok) {
        const errorBody = await response.text();
        throw new Error(`Failed to fetch spendings: ${response.statusText} - ${errorBody}`);
    }

    const data: GroupedSpendingsResponse = await response.json();
    console.log("Fetched Grouped Spendings:", data); // Debug log
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
