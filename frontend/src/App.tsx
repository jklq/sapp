import { useState, useEffect, FormEvent } from 'react';
import { fetchCategories, submitPayment } from './api';
import { Category, PayPayload } from './types';

function App() {
  const [categories, setCategories] = useState<Category[]>([]);
  const [selectedCategory, setSelectedCategory] = useState<string>('');
  const [amount, setAmount] = useState<string>('');
  const [prompt, setPrompt] = useState<string>('');
  const [sharedStatus, setSharedStatus] = useState<PayPayload['shared_status']>('alone');
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);

  useEffect(() => {
    fetchCategories()
      .then(data => {
        setCategories(data);
        if (data.length > 0) {
          setSelectedCategory(data[0].name); // Default to first category
        }
      })
      .catch(err => {
        console.error("Failed to load categories:", err);
        setError('Failed to load categories. Please try again later.');
      });
  }, []); // Empty dependency array means this runs once on mount

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setSuccessMessage(null);

    const numericAmount = parseFloat(amount);
    if (isNaN(numericAmount) || numericAmount <= 0) {
      setError('Please enter a valid positive amount.');
      return;
    }

    if (!selectedCategory) {
      setError('Please select a category.');
      return;
    }

    // Basic prompt validation (optional)
    if (!prompt.trim()) {
        setError('Please enter a description for the purchase.');
        return;
    }


    const payload: PayPayload = {
      amount: numericAmount,
      category: selectedCategory,
      shared_status: sharedStatus,
      prompt: prompt,
    };

    setIsLoading(true);
    try {
      await submitPayment(payload);
      setSuccessMessage('Payment submitted successfully!');
      // Reset form
      setAmount('');
      setPrompt('');
      // Optionally reset category and shared status or keep them
      // setSelectedCategory(categories.length > 0 ? categories[0].name : '');
      // setSharedStatus('alone');
    } catch (err) {
      console.error("Failed to submit payment:", err);
      setError(err instanceof Error ? err.message : 'An unknown error occurred.');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-100 flex items-center justify-center p-4">
      <div className="bg-white shadow-md rounded-lg p-6 w-full max-w-md">
        <h1 className="text-2xl font-bold mb-6 text-center text-gray-700">Log Spending</h1>

        {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
        {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Amount Input */}
          <div>
            <label htmlFor="amount" className="block text-sm font-medium text-gray-700">Amount</label>
            <input
              type="number"
              id="amount"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              placeholder="0.00"
              step="0.01"
              min="0.01"
              required
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
            />
          </div>

          {/* Description/Prompt Input */}
          <div>
            <label htmlFor="prompt" className="block text-sm font-medium text-gray-700">Description</label>
            <input
              type="text"
              id="prompt"
              value={prompt}
              onChange={(e) => setPrompt(e.target.value)}
              placeholder="e.g., Groceries at Rema"
              required
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
            />
          </div>


          {/* Category Select */}
          <div>
            <label htmlFor="category" className="block text-sm font-medium text-gray-700">Category</label>
            <select
              id="category"
              value={selectedCategory}
              onChange={(e) => setSelectedCategory(e.target.value)}
              required
              className="mt-1 block w-full px-3 py-2 border border-gray-300 bg-white rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
              disabled={categories.length === 0}
            >
              {categories.length === 0 && <option>Loading categories...</option>}
              {categories.map(cat => (
                <option key={cat.id} value={cat.name}>{cat.name}</option>
              ))}
            </select>
          </div>

          {/* Shared Status Radio */}
          <div>
            <span className="block text-sm font-medium text-gray-700 mb-1">Shared Status</span>
            <div className="flex items-center space-x-4">
              <label className="inline-flex items-center">
                <input
                  type="radio"
                  name="sharedStatus"
                  value="alone"
                  checked={sharedStatus === 'alone'}
                  onChange={() => setSharedStatus('alone')}
                  className="form-radio h-4 w-4 text-indigo-600"
                />
                <span className="ml-2 text-sm text-gray-700">Alone</span>
              </label>
              <label className="inline-flex items-center">
                <input
                  type="radio"
                  name="sharedStatus"
                  value="shared"
                  checked={sharedStatus === 'shared'}
                  onChange={() => setSharedStatus('shared')}
                  className="form-radio h-4 w-4 text-indigo-600"
                />
                <span className="ml-2 text-sm text-gray-700">Shared</span>
              </label>
               {/* Add 'mix' if/when backend supports it properly via pay endpoint */}
               {/*
               <label className="inline-flex items-center">
                 <input
                   type="radio"
                   name="sharedStatus"
                   value="mix"
                   checked={sharedStatus === 'mix'}
                   onChange={() => setSharedStatus('mix')}
                   className="form-radio h-4 w-4 text-indigo-600"
                 />
                 <span className="ml-2 text-sm text-gray-700">Mix</span>
               </label>
               */}
            </div>
          </div>

          {/* Submit Button */}
          <div>
            <button
              type="submit"
              disabled={isLoading || categories.length === 0}
              className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isLoading || categories.length === 0 ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
            >
              {isLoading ? 'Submitting...' : 'Submit Payment'}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default App;
