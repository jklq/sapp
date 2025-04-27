import { useState, useEffect, FormEvent, useCallback } from 'react';
import { fetchDepositById, updateDeposit } from './api';
import { DepositTemplate, UpdateDepositPayload } from './types';

interface EditDepositPageProps {
    depositId: number;
    onBack: () => void; // Callback to go back to the previous view (e.g., HistoryList)
}

// Helper to format Date object or ISO string to 'YYYY-MM-DD'
const formatDateForInput = (date: Date | string | null | undefined): string => {
    if (!date) return '';
    try {
        const d = typeof date === 'string' ? new Date(date) : date;
        // Adjust for timezone offset to get the correct local date
        const offset = d.getTimezoneOffset();
        const adjustedDate = new Date(d.getTime() - (offset * 60 * 1000));
        return adjustedDate.toISOString().split('T')[0];
    } catch (e) {
        console.error("Error formatting date:", e);
        return '';
    }
};


function EditDepositPage({ depositId, onBack }: EditDepositPageProps) {
    // Form field states
    const [amount, setAmount] = useState<string>('');
    const [description, setDescription] = useState<string>('');
    const [depositDate, setDepositDate] = useState<string>('');
    const [isRecurring, setIsRecurring] = useState<boolean>(false);
    const [recurrencePeriod, setRecurrencePeriod] = useState<string>('monthly');
    const [endDate, setEndDate] = useState<string>(''); // Store as 'YYYY-MM-DD' string

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [isSaving, setIsSaving] = useState<boolean>(false);
    const [error, setError] = useState<string | null>(null);
    const [saveError, setSaveError] = useState<string | null>(null);
    const [successMessage, setSuccessMessage] = useState<string | null>(null);

    // Fetch deposit data
    const loadDepositData = useCallback(() => {
        setIsLoading(true);
        setError(null);
        setSaveError(null);
        setSuccessMessage(null);

        fetchDepositById(depositId)
            .then((data: DepositTemplate) => {
                setAmount(data.amount.toString());
                setDescription(data.description);
                setDepositDate(formatDateForInput(data.date)); // Use 'date' field
                setIsRecurring(data.is_recurring);
                setRecurrencePeriod(data.recurrence_period || 'monthly');
                setEndDate(formatDateForInput(data.end_date));
            })
            .catch(err => {
                console.error(`Failed to fetch deposit ${depositId}:`, err);
                setError(err instanceof Error ? err.message : 'Failed to load deposit details.');
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, [depositId]); // Reload if depositId changes

    useEffect(() => {
        loadDepositData();
    }, [loadDepositData]); // Run effect when loadDepositData changes (i.e., depositId changes)

    const handleSubmit = async (event: FormEvent) => {
        event.preventDefault();
        setSaveError(null);
        setSuccessMessage(null);
        setIsSaving(true);

        const numericAmount = parseFloat(amount);
        if (isNaN(numericAmount) || numericAmount <= 0) {
            setSaveError('Please enter a valid positive amount.');
            setIsSaving(false);
            return;
        }
        if (!description.trim()) {
            setSaveError('Please enter a description.');
            setIsSaving(false);
            return;
        }
        if (!depositDate) {
            setSaveError('Please select a deposit date.');
            setIsSaving(false);
            return;
        }
        if (isRecurring && !recurrencePeriod) {
            setSaveError('Please select the recurrence period for recurring deposits.');
            setIsSaving(false);
            return;
        }
        // Validate end date format if provided
        if (endDate) {
            try {
                new Date(endDate).toISOString();
            } catch (e) {
                setSaveError('Invalid end date format. Use YYYY-MM-DD.');
                setIsSaving(false);
                return;
            }
        }

        // Construct payload with the full state based on the form.
        // The backend handler should ideally fetch the current state and merge,
        // but sending the full state is simpler here.
        const payload: UpdateDepositPayload = {
            amount: numericAmount,
            description: description.trim(),
            deposit_date: depositDate,
            is_recurring: isRecurring,
            // Send period only if recurring, otherwise send null/undefined based on API expectation
            recurrence_period: isRecurring ? recurrencePeriod : null,
            // Send end date as YYYY-MM-DD string, or null if empty
            end_date: endDate || null,
        };

        try {
            const updatedDeposit = await updateDeposit(depositId, payload);
            setSuccessMessage('Deposit updated successfully!');
            // Optionally update form state again with potentially cleaned data from backend response
            setAmount(updatedDeposit.amount.toString());
            setDescription(updatedDeposit.description);
            setDepositDate(formatDateForInput(updatedDeposit.date)); // Use 'date' field
            setIsRecurring(updatedDeposit.is_recurring);
            setRecurrencePeriod(updatedDeposit.recurrence_period || 'monthly');
            setEndDate(formatDateForInput(updatedDeposit.end_date));

            // Navigate back after a short delay
            setTimeout(() => {
                onBack();
            }, 1500);

        } catch (err) {
            console.error(`Failed to update deposit ${depositId}:`, err);
            setSaveError(err instanceof Error ? err.message : 'An unknown error occurred during save.');
        } finally {
            setIsSaving(false);
        }
    };

    if (isLoading) {
        return <div className="text-center p-4">Loading deposit details...</div>;
    }

    if (error) {
        return (
            <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">
                Error loading details: {error}
                <button onClick={onBack} className="ml-4 text-sm text-red-800 underline">Back</button>
            </div>
        );
    }

    return (
        <div className="bg-white shadow-md rounded-lg w-full max-w-md">
            <div className="p-4">
                <div className="flex justify-between items-center mb-4">
                    <h1 className="text-2xl font-bold text-gray-700">Edit Deposit</h1>
                    <button
                        onClick={onBack}
                        className="text-sm text-indigo-600 hover:text-indigo-800"
                    >
                        &larr; Back
                    </button>
                </div>


                {saveError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{saveError}</div>}
                {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

                <form onSubmit={handleSubmit} className="space-y-4">
                    {/* Amount Input */}
                    <div>
                        <label htmlFor="edit-deposit-amount" className="block text-sm font-medium text-gray-700">Amount</label>
                        <input
                            type="number"
                            id="edit-deposit-amount"
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
                        <label htmlFor="edit-deposit-description" className="block text-sm font-medium text-gray-700">Description</label>
                        <input
                            type="text"
                            id="edit-deposit-description"
                            value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="e.g., Salary May, Birthday Gift"
                            required
                            className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>

                    {/* Deposit Date Input */}
                    <div>
                        <label htmlFor="edit-deposit-date" className="block text-sm font-medium text-gray-700">
                            {isRecurring ? 'Start Date' : 'Deposit Date'}
                        </label>
                        <input
                            type="date"
                            id="edit-deposit-date"
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
                                id="edit-is-recurring"
                                name="edit-is-recurring"
                                type="checkbox"
                                checked={isRecurring}
                                onChange={(e) => setIsRecurring(e.target.checked)}
                                className="focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded"
                            />
                        </div>
                        <div className="ml-3 text-sm">
                            <label htmlFor="edit-is-recurring" className="font-medium text-gray-700">Is this recurring?</label>
                        </div>
                    </div>

                    {/* Recurrence Period Select (Conditional) */}
                    {isRecurring && (
                        <div>
                            <label htmlFor="edit-recurrence-period" className="block text-sm font-medium text-gray-700">Recurrence Period</label>
                            <select
                                id="edit-recurrence-period"
                                value={recurrencePeriod}
                                onChange={(e) => setRecurrencePeriod(e.target.value)}
                                required={isRecurring}
                                className="mt-1 block w-full px-3 py-2 border border-gray-300 bg-white rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                            >
                                <option value="weekly">Weekly</option>
                                <option value="monthly">Monthly</option>
                                <option value="yearly">Yearly</option>
                                {/* Add other options if needed */}
                            </select>
                        </div>
                    )}

                    {/* End Date Input (Conditional) */}
                    {isRecurring && (
                        <div>
                            <label htmlFor="edit-end-date" className="block text-sm font-medium text-gray-700">End Date (Optional)</label>
                            <input
                                type="date"
                                id="edit-end-date"
                                value={endDate}
                                onChange={(e) => setEndDate(e.target.value)}
                                className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                            />
                             <p className="mt-1 text-xs text-gray-500">Leave blank for the recurrence to continue indefinitely.</p>
                        </div>
                    )}


                    {/* Submit Button */}
                    <div className="pt-4">
                        <button
                            type="submit"
                            disabled={isSaving || !!successMessage} // Disable while saving or after success
                            className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isSaving || successMessage ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
                        >
                            {isSaving ? 'Saving...' : 'Save Changes'}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
}

export default EditDepositPage;
