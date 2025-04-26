import { Category, PayPayload } from "./types";

// Define the base URL for your backend API
// You might want to use environment variables for this in a real app
const API_BASE_URL = import.meta.env.VITE_API_URL || "http://localhost:3000";

export async function fetchCategories(): Promise<Category[]> {
  const response = await fetch(`${API_BASE_URL}/v1/categories`);
  if (!response.ok) {
    throw new Error(`Failed to fetch categories: ${response.statusText}`);
  }
  return await response.json();
}

// Renamed: Submits a payment with a manually selected category
export async function submitManualPayment(payload: PayPayload): Promise<void> {
  // This function uses the existing backend endpoint which expects category in the URL
  const { shared_status, amount, category } = payload;
  // Ensure category is provided for manual submission
  if (!category) {
    throw new Error("Category is required for manual payment submission.");
  }
  const url = `${API_BASE_URL}/v1/pay/${shared_status}/${amount}/${encodeURIComponent(category)}`;

  const response = await fetch(url, {
    method: "POST",
    headers: {
      // No Content-Type needed for POST without body
    },
    // No body needed for this specific endpoint based on backend/pay/pay.go
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


// New function: Submits data to trigger AI categorization
export async function submitAICategorization(payload: AICategorizationPayload): Promise<void> {
  // TODO: Define the actual backend endpoint for AI categorization.
  // This endpoint should accept amount, prompt, and shared_status, likely in the request body.
  // Example endpoint: POST /v1/categorize
  const url = `${API_BASE_URL}/v1/categorize`; // Placeholder URL

  // Assuming the backend endpoint for AI categorization expects a JSON body
  const response = await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      // Add authentication headers if needed
    },
    body: JSON.stringify(payload),
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
}
