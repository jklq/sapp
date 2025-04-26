import { useState, useEffect, FormEvent, useMemo } from 'react';
import { fetchCategories, submitManualPayment, submitAICategorization } from './api';
import { Category, PayPayload, AICategorizationPayload } from './types';

type Mode = 'ai' | 'manual';

function App() {
  // Mode state
  const [mode, setMode] = useState<Mode>('ai'); // Default to AI mode

  // Form field states
  const [categories, setCategories] = useState<Category[]>([]);
  const [selectedCategory, setSelectedCategory] = useState<string>('');
  const [amount, setAmount] = useState<string>('');
  const [prompt, setPrompt] = useState<string>(''); // Used in AI mode
  const [sharedStatus, setSharedStatus] = useState<PayPayload['shared_status']>('alone');

  // UI states
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [successMessage, setSuccessMessage] = useState<string | null>(null);
  const [isFetchingCategories, setIsFetchingCategories] = useState<boolean>(true);

  // Fetch categories on mount, needed for Manual mode
  useEffect(() => {
    setIsFetchingCategories(true);
    fetchCategories()
      .then(data => {
        setCategories(data);
        // Set default category selection only if categories are loaded
        if (data.length > 0) {
          setSelectedCategory(data[0].name);
        } else {
          setSelectedCategory(''); // Ensure no category is selected if fetch fails or returns empty
        }
      })
      .catch(err => {
        console.error("Failed to load categories:", err);
        // Set error state specific to category loading, maybe display differently
        setError('Failed to load categories. Manual mode will be unavailable.');
        setCategories([]); // Ensure categories is empty on error
        setSelectedCategory('');
      })
      .finally(() => {
        setIsFetchingCategories(false);
      });
  }, []); // Empty dependency array means this runs once on mount

  // Reset specific fields when mode changes
  useEffect(() => {
    // When switching to AI, clear selected category (it's not used)
    // When switching to Manual, clear prompt (it's not used)
    if (mode === 'ai') {
       // Keep prompt, clear category if needed (though it's hidden)
       // setSelectedCategory(''); // Optional: clear category state
    } else { // mode === 'manual'
      setPrompt(''); // Clear prompt when switching to manual
      // Ensure a category is selected if possible
      if (categories.length > 0 && !selectedCategory) {
        setSelectedCategory(categories[0].name);
      }
    }
    // Clear errors/success messages on mode switch
    setError(null);
    setSuccessMessage(null);
  }, [mode, categories]); // Rerun when mode or categories change

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setSuccessMessage(null);

    const numericAmount = parseFloat(amount);
    if (isNaN(numericAmount) || numericAmount <= 0) {
      setError('Please enter a valid positive amount.');
      return;
    }

    // Mode-specific validation and payload creation
    setIsLoading(true);
    try {
      if (mode === 'ai') {
        // AI Mode Validation
        if (!prompt.trim()) {
          setError('Please enter a description for the AI to categorize.');
          setIsLoading(false);
          return;
        }

        const aiPayload: AICategorizationPayload = {
          amount: numericAmount,
          shared_status: sharedStatus,
          prompt: prompt,
        };
        console.log("Submitting AI Payload:", aiPayload); // Debug log
        await submitAICategorization(aiPayload);
        setSuccessMessage('Spending submitted for AI categorization!');
        // Reset form for AI mode
        setAmount('');
        setPrompt('');
        // Optionally reset shared status or keep it
        // setSharedStatus('alone');

      } else { // mode === 'manual'
        // Manual Mode Validation
        if (!selectedCategory) {
          setError('Please select a category.');
          setIsLoading(false);
          return;
        }

        const manualPayload: PayPayload = {
          amount: numericAmount,
          category: selectedCategory,
          shared_status: sharedStatus,
          prompt: '', // Prompt is not used in manual mode submission
        };
        console.log("Submitting Manual Payload:", manualPayload); // Debug log
        await submitManualPayment(manualPayload);
        setSuccessMessage('Manual payment submitted successfully!');
        // Reset form for Manual mode
        setAmount('');
        // Keep selected category and shared status? Or reset?
        // setSelectedCategory(categories.length > 0 ? categories[0].name : '');
        // setSharedStatus('alone');
      }
    } catch (err) {
      console.error(`Failed to submit in ${mode} mode:`, err);
      setError(err instanceof Error ? err.message : 'An unknown error occurred.');
    } finally {
      setIsLoading(false);
    }
  };

   // Determine if submit button should be disabled
   const isSubmitDisabled = useMemo(() => {
    if (isLoading) return true;
    if (mode === 'manual' && (isFetchingCategories || categories.length === 0)) return true;
    // Add other conditions if needed (e.g., required fields not filled)
    return false;
  }, [isLoading, mode, isFetchingCategories, categories.length]);


  return (
    <div className="min-h-screen bg-gray-100 flex items-center justify-center p-4">
      <div className="bg-white shadow-md rounded-lg p-6 w-full max-w-md">
        <h1 className="text-2xl font-bold mb-4 text-center text-gray-700">Log Spending</h1>

        {/* Mode Switcher (Tabs) */}
        <div className="mb-6 flex justify-center border-b border-gray-200">
          <button
            onClick={() => setMode('ai')}
            className={`py-2 px-4 text-sm font-medium text-center rounded-t-lg ${mode === 'ai' ? 'border-b-2 border-indigo-500 text-indigo-600' : 'text-gray-500 hover:text-gray-700 hover:border-gray-300'}`}
          >
            Auto (AI)
          </button>
          <button
            onClick={() => setMode('manual')}
            className={`py-2 px-4 text-sm font-medium text-center rounded-t-lg ${mode === 'manual' ? 'border-b-2 border-indigo-500 text-indigo-600' : 'text-gray-500 hover:text-gray-700 hover:border-gray-300'}`}
            disabled={isFetchingCategories && categories.length === 0} // Disable if categories fail to load
          >
            Manual
          </button>
        </div>


        {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
        {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Amount Input (Common) */}
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

          {/* Conditional Fields based on Mode */}
          {mode === 'ai' && (
            <div>
              <label htmlFor="prompt" className="block text-sm font-medium text-gray-700">Description (for AI)</label>
              <input
                type="text"
                id="prompt"
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="e.g., Groceries at Rema, train ticket"
                required
                className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
              />
            </div>
          )}

          {mode === 'manual' && (
            <div>
              <label htmlFor="category" className="block text-sm font-medium text-gray-700">Category</label>
              <select
                id="category"
                value={selectedCategory}
                onChange={(e) => setSelectedCategory(e.target.value)}
                required
                className="mt-1 block w-full px-3 py-2 border border-gray-300 bg-white rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                disabled={isFetchingCategories || categories.length === 0}
              >
                {isFetchingCategories ? (
                   <option>Loading categories...</option>
                ) : categories.length === 0 ? (
                   <option>No categories available</option>
                ) : (
                  categories.map(cat => (
                    <option key={cat.id} value={cat.name}>{cat.name}</option>
                  ))
                )}
              </select>
            </div>
          )}

          {/* Shared Status Radio (Only for Manual Mode) */}
          {mode === 'manual' && (
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
          )}
                <input
                  type="radio"
                  name="sharedStatus"
                  value="alone"
                  checked={sharedStatus === 'alone'}
                  onChange={() => setSharedStatus('alone')}
                  className="form-radio h-4 w-4 text-indigo-600"
                />

          {/* Submit Button (Common) */}
          <div>
            <button
              type="submit"
              disabled={isSubmitDisabled}
              className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isSubmitDisabled ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
            >
              {isLoading ? 'Submitting...' : (mode === 'ai' ? 'Submit for AI Categorization' : 'Submit Manual Payment')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

export default App;
