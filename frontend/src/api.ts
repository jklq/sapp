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

export async function submitPayment(payload: PayPayload): Promise<void> {
  // TODO: The backend /v1/pay endpoint currently only takes shared_status, amount, and category name in the URL path.
  // It doesn't accept a request body or the 'prompt'.
  // This needs reconciliation. For now, we'll call the endpoint as defined,
  // but the 'prompt' won't be sent. The backend might need changes
  // to accept the prompt and potentially trigger the AI categorization job.

  const { shared_status, amount, category } = payload;
  const url = `${API_BASE_URL}/v1/pay/${shared_status}/${amount}/${category}`;

  const response = await fetch(url, {
    method: "POST",
    headers: {
      // No Content-Type needed for POST without body
    },
    // body: JSON.stringify(payload) // If the backend expected a body
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

  // No content expected on success based on backend code
}
