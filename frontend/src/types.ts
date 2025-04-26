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
