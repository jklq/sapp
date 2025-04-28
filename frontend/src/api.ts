import {
  Category,
  PayPayload,
  AICategorizationPayload,
  LoginPayload,
  LoginResponse,
  PartnerRegistrationPayload,
  PartnerRegistrationResponse,
  AddDepositPayload,
  AddDepositResponse,
  UpdateSpendingPayload,
  TransferStatusResponse,
  HistoryResponse,
  DepositTemplate,
  UpdateDepositPayload,
  DeleteDepositResponse,
  CategorySpendingStat, // Import the type needed for the new function
} from "./types";

// --- Constants ---
const AUTH_TOKEN_KEY = "authToken";
// Use environment variable for API base URL, fallback for development
const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL || "http://localhost:3000";

// --- Auth Token Helpers ---

export function storeToken(token: string): void {
  localStorage.setItem(AUTH_TOKEN_KEY, token);
}

export function getToken(): string | null {
  return localStorage.getItem(AUTH_TOKEN_KEY);
}

export function removeToken(): void {
  localStorage.removeItem(AUTH_TOKEN_KEY);
}

// --- Core Fetch Wrapper ---

// Wrapper for fetch that automatically adds Authorization header if token exists
async function fetchWithAuth(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  const token = getToken();
  const headers = new Headers(options.headers || {});

  if (token) {
    // Prepend "Bearer " to the token for standard JWT authorization
    headers.set("Authorization", `Bearer ${token}`);
  }
  // Ensure Content-Type is set for methods that have a body, unless it's FormData
  if (
    options.body &&
    !(options.body instanceof FormData) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
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
  const url = `${API_BASE_URL}/v1/pay`;

  const response = await fetchWithAuth(url, {
    method: "POST",
    body: JSON.stringify(payload),
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

export async function submitAICategorization(
  payload: AICategorizationPayload
): Promise<void> {
  const url = `${API_BASE_URL}/v1/categorize`;

  const response = await fetchWithAuth(url, {
    method: "POST",
    body: JSON.stringify(payload),
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
    console.warn(
      `Unexpected status code from AI categorization endpoint: ${response.status}`,
      responseBody
    );
    // Optionally throw an error or handle differently based on other success codes if needed
    throw new Error(
      `AI categorization submission failed with status: ${response.status} - ${responseBody}`
    );
  }

  // Handle response if needed (e.g., getting a job ID back)
  // The backend currently returns the job_id in the body on 202
  // You might want to parse and return this if the frontend needs it
  // const data = await response.json();
  // return data.job_id;
}

// --- Auth API Functions ---

export async function loginUser(payload: LoginPayload): Promise<LoginResponse> {
  const url = `${API_BASE_URL}/v1/login`;

  const response = await fetch(url, {
    // Login doesn't need auth token initially
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
    } catch /* (e) */ {
      // Remove unused 'e'
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

export async function registerPartners(
  payload: PartnerRegistrationPayload
): Promise<PartnerRegistrationResponse> {
  const url = `${API_BASE_URL}/v1/register/partners`;

  const response = await fetch(url, {
    // Registration is public, no auth token needed
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
    } catch /* (e) */ {
      // Remove unused 'e'
      try {
        // If not JSON, try reading as text
        const textError = await response.text();
        if (textError) {
          errorBody += ` - ${textError}`;
        }
      } catch /* (textErr) */ {
        // Remove unused 'textErr'
        // Ignore if reading text also fails
      }
    }
    throw new Error(errorBody);
  }

  // Expecting 201 Created with JSON body on success
  if (response.status !== 201) {
    console.warn(
      `Unexpected status code after partner registration: ${response.status}`
    );
    // Optionally throw error if status is not 201
  }

  const data: PartnerRegistrationResponse = await response.json();
  return data;
}

// --- Deposit API Functions ---

export async function addDeposit(
  payload: AddDepositPayload
): Promise<AddDepositResponse> {
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
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 201 Created with JSON body on success
  if (response.status !== 201) {
    console.warn(
      `Unexpected status code after adding deposit: ${response.status}`
    );
  }

  const data: AddDepositResponse = await response.json();
  return data;
}

export async function fetchDepositById(
  depositId: number
): Promise<DepositTemplate> {
  const url = `${API_BASE_URL}/v1/deposits/${depositId}`;
  const response = await fetchWithAuth(url);

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to fetch deposit details: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expect DepositTemplate structure
  const data: DepositTemplate = await response.json();
  console.log("Fetched Deposit Details:", data);
  return data;
}

export async function updateDeposit(
  depositId: number,
  payload: UpdateDepositPayload
): Promise<DepositTemplate> {
  const url = `${API_BASE_URL}/v1/deposits/${depositId}`;
  const response = await fetchWithAuth(url, {
    method: "PUT",
    body: JSON.stringify(payload),
    // fetchWithAuth will set Content-Type: application/json
  });

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to update deposit: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 200 OK with JSON body containing the updated deposit
  const data: { message: string; deposit: DepositTemplate } =
    await response.json();
  return data.deposit;
}

export async function deleteDeposit(
  depositId: number
): Promise<DeleteDepositResponse> {
  const url = `${API_BASE_URL}/v1/deposits/${depositId}`;
  const response = await fetchWithAuth(url, {
    method: "DELETE",
  });

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to delete deposit: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 200 OK with JSON body on success
  const data: DeleteDepositResponse = await response.json();
  return data;
}

// --- History API Functions ---

export async function fetchHistory(): Promise<HistoryResponse> {
  const url = `${API_BASE_URL}/v1/history`;
  const response = await fetchWithAuth(url);

  if (!response.ok) {
    const errorBody = await response.text();
    throw new Error(
      `Failed to fetch history: ${response.statusText} - ${errorBody}`
    );
  }

  const data: HistoryResponse = await response.json();
  console.log("Fetched History:", data);
  return data;
}

export async function updateSpendingItem(
  spendingId: number,
  payload: UpdateSpendingPayload
): Promise<void> {
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
    } catch /* (e) */ {
      // Remove unused 'e'
      // Use text if not JSON
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 200 OK with no body on success
  if (response.status !== 200) {
    console.warn(
      `Unexpected status code after updating spending item: ${response.status}`
    );
    // Optionally handle other success codes if the backend changes
  }
}

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
    } catch /* (e) */ {
      // Remove unused 'e'
      // Use text if not JSON
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 204 No Content on success
  if (response.status !== 204) {
    console.warn(
      `Unexpected status code after deleting job: ${response.status}`
    );
    // Optionally handle other success codes if the backend changes
  }
}

// --- Stats API Functions ---

export async function fetchLastMonthSpendingStats(): Promise<CategorySpendingStat[]> {
    const url = `${API_BASE_URL}/v1/stats/spending/last-month`;
    const response = await fetchWithAuth(url); // Use GET by default

    if (!response.ok) {
        const errorBody = await response.text();
        let errorMessage = `Failed to fetch spending stats: ${response.statusText}`;
        try {
            const errData = JSON.parse(errorBody);
            errorMessage = errData.message || errData.error || errorMessage;
        } catch /* (e) */ {
            errorMessage += ` - ${errorBody}`;
        }
        throw new Error(errorMessage);
    }

    const data: CategorySpendingStat[] = await response.json();
    console.log("Fetched Spending Stats:", data);
    return data;
}


// --- Transfer API Functions ---

export async function fetchTransferStatus(): Promise<TransferStatusResponse> {
  const url = `${API_BASE_URL}/v1/transfer/status`;
  const response = await fetchWithAuth(url);

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to fetch transfer status: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  const data: TransferStatusResponse = await response.json();
  console.log("Fetched Transfer Status:", data);
  return data;
}

export async function recordTransfer(): Promise<void> {
  const url = `${API_BASE_URL}/v1/transfer/record`;
  const response = await fetchWithAuth(url, { method: "POST" });

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to record transfer: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      // Remove unused 'e'
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Expecting 200 OK on success
  if (response.status !== 200) {
    console.warn(
      `Unexpected status code after recording transfer: ${response.status}`
    );
  }
}
