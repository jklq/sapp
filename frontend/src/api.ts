import {
  Category,
  PayPayload,
  AICategorizationPayload,
  LoginPayload,
  LoginResponse,
  RefreshTokenRequest, // Added
  RefreshTokenResponse, // Added
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
  CategorySpendingStat,
  DepositStatsResponse, // Import the type for deposit stats
} from "./types";

// --- Constants ---
const ACCESS_TOKEN_KEY = "accessToken"; // Renamed
const REFRESH_TOKEN_KEY = "refreshToken"; // Added
// Use environment variable for API base URL, fallback for development
const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL || "http://localhost:3000";

// --- Auth Token Helpers ---

export function storeTokens(accessToken: string, refreshToken: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, accessToken);
  localStorage.setItem(REFRESH_TOKEN_KEY, refreshToken);
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function removeTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

// --- Core Fetch Wrapper ---

// Flag to prevent infinite refresh loops
let isRefreshing = false;
// Queue for requests that arrive while token is being refreshed
let refreshSubscribers: ((token: string | null) => void)[] = [];

// Function to add request callbacks to the queue
const subscribeTokenRefresh = (cb: (token: string | null) => void) => {
  refreshSubscribers.push(cb);
};

// Function to notify all queued requests with the new token (or null if refresh failed)
const onRefreshed = (token: string | null) => {
  refreshSubscribers.forEach((cb) => cb(token));
  // Clear the queue after notifying
  refreshSubscribers = [];
};


// Wrapper for fetch that automatically adds Authorization header and handles token refresh
async function fetchWithAuth(
  url: string,
  options: RequestInit = {}
): Promise<Response> {
  // Check if currently refreshing. If so, queue this request.
  if (isRefreshing) {
    return new Promise((resolve) => {
      subscribeTokenRefresh((newToken: string | null) => {
        // When refresh is done, retry the request with the new token
        if (newToken) {
          const newOptions = { ...options };
          const headers = new Headers(newOptions.headers || {});
          headers.set("Authorization", `Bearer ${newToken}`);
          newOptions.headers = headers;
          resolve(fetch(url, newOptions)); // Re-run fetch, not fetchWithAuth to avoid loop
        } else {
          // Refresh failed, reject the promise (or handle as needed)
          // This case should ideally trigger logout in the refresh logic itself
          reject(new Error("Token refresh failed, unable to proceed."));
        }
      });
    });
  }

  const accessToken = getAccessToken();
  const headers = new Headers(options.headers || {});

  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  // Ensure Content-Type is set for methods that have a body, unless it's FormData
  if (
    options.body &&
    !(options.body instanceof FormData) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json");
  }

  // Execute the fetch request
  const response = await fetch(url, {
    ...options,
    headers: headers,
  });

  // Check for unauthorized status (token expired/invalid)
  if (response.status === 401 && !options.headers?.has("X-Skip-Refresh")) { // Check for 401 and ensure we aren't already refreshing
    const currentRefreshToken = getRefreshToken();
    if (!currentRefreshToken) {
      console.error("401 received but no refresh token found. Logging out.");
      removeTokens();
      window.location.href = '/login'; // Redirect to login
      throw new Error("Unauthorized: No refresh token available.");
    }

    // Prevent multiple refresh attempts simultaneously
    if (!isRefreshing) {
        isRefreshing = true;
        try {
            console.log("Access token expired or invalid. Attempting refresh...");
            const refreshResponse = await refreshToken(currentRefreshToken);
            storeTokens(refreshResponse.access_token, currentRefreshToken); // Store new access token (and potentially new refresh token if rotation is implemented)
            console.log("Token refresh successful.");

            // Notify queued requests
            onRefreshed(refreshResponse.access_token);

            // Retry the original request with the new token
            headers.set("Authorization", `Bearer ${refreshResponse.access_token}`);
            // Add a header to prevent refresh loop if the refresh endpoint itself returns 401
            // (though it shouldn't if the refresh token is valid)
            const retryOptions = { ...options, headers };
            isRefreshing = false; // Reset flag before retrying
            return fetch(url, retryOptions); // Re-run fetch directly

        } catch (refreshError) {
            console.error("Token refresh failed:", refreshError);
            isRefreshing = false; // Reset flag on failure
            onRefreshed(null); // Notify queued requests that refresh failed
            removeTokens(); // Clear invalid tokens
            window.location.href = '/login'; // Redirect to login
            // Throw error to prevent further processing in the original caller
            throw new Error(`Unauthorized: Token refresh failed. ${refreshError}`);
        } finally {
           // Ensure flag is reset even if unexpected errors occur
           isRefreshing = false;
        }
    } else {
      // If already refreshing, queue this request (handled at the start of the function)
      return new Promise((resolve, reject) => { // Added reject here
        subscribeTokenRefresh((newToken: string | null) => {
          if (newToken) {
            const newOptions = { ...options };
            const headers = new Headers(newOptions.headers || {});
            headers.set("Authorization", `Bearer ${newToken}`);
            newOptions.headers = headers;
            resolve(fetch(url, newOptions)); // Re-run fetch
          } else {
            reject(new Error("Token refresh failed while request was queued."));
          }
        });
      });
    }
  } else if (!response.ok && response.status !== 401) {
    // Handle other non-401 errors immediately
    // We might want to parse the error body here as well
    console.error(`HTTP error! status: ${response.status}`, await response.text());
    // Throw an error or handle as appropriate for non-auth errors
    // throw new Error(`HTTP error! status: ${response.status}`);
  }

  return response; // Return the original response if not 401 or other handled error
}

// --- API Function for Refreshing Token ---
async function refreshToken(refreshTokenValue: string): Promise<RefreshTokenResponse> {
    const url = `${API_BASE_URL}/v1/refresh`;
    const payload: RefreshTokenRequest = { refresh_token: refreshTokenValue };

    const response = await fetch(url, { // Use plain fetch, no auth needed for refresh endpoint
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
            // Add a header to signal this is the refresh request itself,
            // preventing fetchWithAuth from trying to refresh again if this fails with 401
            'X-Skip-Refresh': 'true',
        },
        body: JSON.stringify(payload),
    });

    if (!response.ok) {
        // If refresh fails, backend should ideally return a specific status code or error message
        const errorBody = await response.text();
        console.error("Refresh token request failed:", response.status, errorBody);
        // Throw specific error to be caught by the fetchWithAuth retry logic
        throw new Error(`Failed to refresh token: ${response.status} - ${errorBody}`);
    }

    const data: RefreshTokenResponse = await response.json();
    if (!data.access_token) {
        throw new Error("Refresh successful, but no new access token received.");
    }
    return data;
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

  // Assuming the backend returns JSON with access_token, refresh_token, user_id, first_name
  const data: LoginResponse = await response.json();
  if (!data.access_token || !data.refresh_token) {
    throw new Error("Login successful, but tokens not received.");
  }
  // Store both tokens
  storeTokens(data.access_token, data.refresh_token);

  // Return the full response data (excluding tokens if preferred, but usually includes user info)
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

// Fetches spending stats for a given date range.
// Dates should be in "YYYY-MM-DD" format.
export async function fetchSpendingStats(
  startDate: string,
  endDate: string
): Promise<CategorySpendingStat[]> {
  // Validate date format roughly (more robust validation can be added)
  const dateRegex = /^\d{4}-\d{2}-\d{2}$/;
  if (!dateRegex.test(startDate) || !dateRegex.test(endDate)) {
    throw new Error("Invalid date format. Use YYYY-MM-DD.");
  }

  // Construct URL with query parameters
  const url = new URL(`${API_BASE_URL}/v1/stats/spending`);
  url.searchParams.append("startDate", startDate);
  url.searchParams.append("endDate", endDate);

  const response = await fetchWithAuth(url.toString()); // Use GET by default

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

// Fetches deposit stats for a given date range.
// Dates should be in "YYYY-MM-DD" format.
export async function fetchDepositStats(
  startDate: string,
  endDate: string
): Promise<DepositStatsResponse> {
  const dateRegex = /^\d{4}-\d{2}-\d{2}$/;
  if (!dateRegex.test(startDate) || !dateRegex.test(endDate)) {
    throw new Error("Invalid date format. Use YYYY-MM-DD.");
  }

  const url = new URL(`${API_BASE_URL}/v1/stats/deposits`);
  url.searchParams.append("startDate", startDate);
  url.searchParams.append("endDate", endDate);

  const response = await fetchWithAuth(url.toString());

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to fetch deposit stats: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  const data: DepositStatsResponse = await response.json();
  console.log("Fetched Deposit Stats:", data);
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

// --- Export API Function ---

export async function exportAllData(): Promise<void> {
  const url = `${API_BASE_URL}/v1/export/all`;
  const response = await fetchWithAuth(url); // GET request

  if (!response.ok) {
    const errorBody = await response.text();
    let errorMessage = `Failed to initiate export: ${response.statusText}`;
    try {
      const errData = JSON.parse(errorBody);
      errorMessage = errData.message || errData.error || errorMessage;
    } catch /* (e) */ {
      errorMessage += ` - ${errorBody}`;
    }
    throw new Error(errorMessage);
  }

  // Handle the file download
  try {
    const blob = await response.blob();
    // Extract filename from Content-Disposition header, fallback to a default name
    const contentDisposition = response.headers.get("Content-Disposition");
    let filename = "sapp_export.json"; // Default filename
    if (contentDisposition) {
      const filenameMatch = contentDisposition.match(/filename="?(.+)"?/i);
      if (filenameMatch && filenameMatch.length > 1) {
        filename = filenameMatch[1];
      }
    }

    // Create a temporary link to trigger the download
    const link = document.createElement("a");
    link.href = window.URL.createObjectURL(blob);
    link.download = filename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    window.URL.revokeObjectURL(link.href); // Clean up the object URL
    console.log("Export file download initiated:", filename);
  } catch (err) {
    console.error("Error processing export file download:", err);
    throw new Error("Failed to process the downloaded export file.");
  }
}
