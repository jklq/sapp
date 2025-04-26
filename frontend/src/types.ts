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
