import { useState, FormEvent } from 'react';
import { addDeposit } from './api';
import { AddDepositPayload } from './types';

function AddDepositForm() {
    // Form field states
    const [amount, setAmount] = useState<string>('');
    const [description, setDescription] = useState<string>('');
    const [depositDate, setDepositDate] = useState<string>(new Date().toISOString().split('T')[0]); // Default to today
    const [isRecurring, setIsRecurring] = useState<boolean>(false);
    const [recurrencePeriod, setRecurrencePeriod] = useState<string>(''); // e.g., 'monthly', 'weekly'

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);
    const [successMessage, setSuccessMessage] = useState<string | null>(null);

    const handleSubmit = async (event: FormEvent) => {
        event.preventDefault();
        setError(null);
        setSuccessMessage(null);
        setIsLoading(true);

        const numericAmount = parseFloat(amount);
        if (isNaN(numericAmount) || numericAmount <= 0) {
            setError('Please enter a valid positive amount.');
            setIsLoading(false);
            return;
        }
        if (!description.trim()) {
            setError('Please enter a description.');
            setIsLoading(false);
            return;
        }
        if (!depositDate) {
            setError('Please select a deposit date.');
            setIsLoading(false);
            return;
        }
        if (isRecurring && !recurrencePeriod.trim()) {
            setError('Please specify the recurrence period (e.g., monthly, weekly) for recurring deposits.');
            setIsLoading(false);
            return;
        }

        const payload: AddDepositPayload = {
            amount: numericAmount,
            description: description.trim(),
            deposit_date: depositDate,
            is_recurring: isRecurring,
            recurrence_period: isRecurring ? recurrencePeriod.trim() : null,
        };

        try {
            const result = await addDeposit(payload);
            setSuccessMessage(`${result.message} (ID: ${result.deposit_id})`);
            // Reset form
            setAmount('');
            setDescription('');
            setDepositDate(new Date().toISOString().split('T')[0]); // Reset date to today
            setIsRecurring(false);
            setRecurrencePeriod('');
        } catch (err) {
            console.error(`Failed to add deposit:`, err);
            setError(err instanceof Error ? err.message : 'An unknown error occurred.');
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="bg-white shadow-md rounded-lg w-full max-w-md">
            <div className="p-4">
                <h1 className="text-2xl font-bold mb-4 text-center text-gray-700">Add Deposit / Income</h1>

                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
                {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

                <form onSubmit={handleSubmit} className="space-y-4">
                    {/* Amount Input */}
                    <div>
                        <label htmlFor="deposit-amount" className="block text-sm font-medium text-gray-700">Amount</label>
                        <input
                            type="number"
                            id="deposit-amount"
                            value={amount}
                            onChange={(e) => setAmount(e.target.value)}
                            placeholder="0.00"
                            step="0.01"
                            min="0.01"
                            required
                            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>

                    {/* Description Input */}
                    <div>
                        <label htmlFor="deposit-description" className="block text-sm font-medium text-gray-700">Description</label>
                        <input
                            type="text"
                            id="deposit-description"
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="e.g., Salary May, Birthday Gift"
                            required
                            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>

                    {/* Deposit Date Input */}
                    <div>
                        <label htmlFor="deposit-date" className="block text-sm font-medium text-gray-700">Deposit Date</label>
                        <input
                            type="date"
                            id="deposit-date"
                            value={depositDate}
                            onChange={(e) => setDepositDate(e.target.value)}
                            required
                            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>

                    {/* Recurring Checkbox */}
                    <div className="flex items-start">
                        <div className="flex items-center h-5">
                            <input
                                id="is-recurring"
                                name="is-recurring"
                                type="checkbox"
                                checked={isRecurring}
                                onChange={(e) => setIsRecurring(e.target.checked)}
                                className="focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded"
                            />
                        </div>
                        <div className="ml-3 text-sm">
                            <label htmlFor="is-recurring" className="font-medium text-gray-700">Is this recurring?</label>
                        </div>
                    </div>

                    {/* Recurrence Period Input (Conditional) */}
                    {isRecurring && (
                        <div>
                            <label htmlFor="recurrence-period" className="block text-sm font-medium text-gray-700">Recurrence Period</label>
                            <input
                                type="text"
                                id="recurrence-period"
                                value={recurrencePeriod}
                                onChange={(e) => setRecurrencePeriod(e.target.value)}
                                placeholder="e.g., monthly, weekly, yearly"
                                required={isRecurring} // Required only if checkbox is checked
                                className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                            />
                        </div>
                    )}

                    {/* Submit Button */}
                    <div className="pt-4">
                        <button
                            type="submit"
                            disabled={isLoading}
                            className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isLoading ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
                        >
                            {isLoading ? 'Adding...' : 'Add Deposit'}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
}

export default AddDepositForm;
