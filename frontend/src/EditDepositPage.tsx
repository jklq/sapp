import { useState, useEffect, FormEvent, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom'; // Assuming React Router is used for navigation
import { fetchDepositById, updateDeposit } from './api';
import { DepositTemplate, UpdateDepositPayload } from './types';

// Helper to format date to YYYY-MM-DD for input[type=date]
const formatDateForInput = (date: Date | string | null | undefined): string => {
    if (!date) return '';
    try {
        const d = typeof date === 'string' ? new Date(date) : date;
        // Check if 'd' is a valid Date object after potential parsing
        if (isNaN(d.getTime())) {
             console.error("Invalid date value received:", date);
             return '';
        }

        // Directly extract year, month, day in local time
        const year = d.getFullYear();
        const month = (d.getMonth() + 1).toString().padStart(2, '0'); // getMonth is 0-indexed
        const day = d.getDate().toString().padStart(2, '0');

        return `${year}-${month}-${day}`;
    } catch (e) {
        console.error("Error formatting date:", e);
        return '';
    }
};


interface EditDepositPageProps {
    depositId: number;
    onBack: () => void; // Function to go back to the previous view (e.g., HistoryList)
}

function EditDepositPage({ depositId, onBack }: EditDepositPageProps) {
    // Form field states (initialize empty, fetch data in useEffect)
    const [amount, setAmount] = useState<string>('');
    const [description, setDescription] = useState<string>('');
    // Removed depositDate state
    const [isRecurring, setIsRecurring] = useState<boolean>(false);
    const [recurrencePeriod, setRecurrencePeriod] = useState<string>('monthly');
    const [endDate, setEndDate] = useState<string>(''); // YYYY-MM-DD or empty

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(true); // Loading initial data
    const [isSaving, setIsSaving] = useState<boolean>(false); // Saving changes
    const [isEndingNow, setIsEndingNow] = useState<boolean>(false); // State for "End Now" action
    const [error, setError] = useState<string | null>(null);
    const [successMessage, setSuccessMessage] = useState<string | null>(null);
    const [originalDeposit, setOriginalDeposit] = useState<DepositTemplate | null>(null);

    // Fetch deposit details
    const loadDeposit = useCallback(() => {
        setIsLoading(true);
        setError(null);
        fetchDepositById(depositId)
            .then(data => {
                setOriginalDeposit(data);

                // --- Debugging Date Handling ---
                console.log("Raw data.date from API:", data.date);
                console.log("Raw data.end_date from API:", data.end_date);

                let depositDateObj: Date | null = null;
                if (data.date) {
                    try {
                        depositDateObj = new Date(data.date);
                        if (isNaN(depositDateObj.getTime())) { // Check if date is valid
                            console.error("Invalid deposit date received from API:", data.date);
                            depositDateObj = null;
                        } else {
                             console.log("Parsed depositDateObj (local time):", depositDateObj, depositDateObj.toLocaleDateString());
                        }
                    } catch (e) {
                        console.error("Error parsing deposit date:", data.date, e);
                    }
                }

                let endDateObj: Date | null = null;
                if (data.end_date) {
                    try {
                        endDateObj = new Date(data.end_date);
                         if (isNaN(endDateObj.getTime())) { // Check if date is valid
                            console.error("Invalid end date received from API:", data.end_date);
                            endDateObj = null;
                        } else {
                            console.log("Parsed endDateObj (local time):", endDateObj, endDateObj.toLocaleDateString());
                        }
                    } catch (e) {
                        console.error("Error parsing end date:", data.end_date, e);
                    }
                }
                // --- End Debugging ---

                // Pre-fill form fields using the potentially corrected Date objects
                const formattedDepositDate = formatDateForInput(depositDateObj); // Pass Date object or null
                const formattedEndDate = formatDateForInput(endDateObj); // Pass Date object or null

                console.log("Formatted deposit date for input value:", formattedDepositDate); // Log formatted date
                console.log("Formatted end date for input value:", formattedEndDate);

                setAmount(data.amount.toString());
                setDescription(data.description);
                // Removed setDepositDate call
                setIsRecurring(data.is_recurring);
                setRecurrencePeriod(data.recurrence_period || 'monthly');
                setEndDate(formattedEndDate); // Set the formatted date
            })
            .catch(err => {
                console.error("Failed to load deposit details:", err);
                setError(err instanceof Error ? err.message : 'Failed to load deposit details.');
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, [depositId]);

    useEffect(() => {
        loadDeposit();
    }, [loadDeposit]);

    const handleSubmit = async (event: FormEvent) => {
        event.preventDefault();
        setError(null);
        setSuccessMessage(null);
        setIsSaving(true);

        const numericAmount = parseFloat(amount);
        if (isNaN(numericAmount) || numericAmount <= 0) {
            setError('Please enter a valid positive amount.');
            setIsSaving(false);
            return;
        }
        if (!description.trim()) {
            setError('Please enter a description.');
            setIsSaving(false);
            return;
        }
        // Removed depositDate validation
        if (isRecurring && !recurrencePeriod) {
            setError('Please select the recurrence period for recurring deposits.');
            setIsSaving(false);
            return;
        }
        // Validate end date is not before original deposit date if both are set
        const originalDepositDateStr = originalDeposit ? formatDateForInput(originalDeposit.date) : null;
        if (endDate && originalDepositDateStr && endDate < originalDepositDateStr) {
            setError('End date cannot be before the original deposit start date.');
            setIsSaving(false);
            return;
        }


        // Construct payload with only changed fields (optional optimization)
        // For simplicity, we send all fields for now. Backend handles partial updates if needed.
        const payload: UpdateDepositPayload = {
            amount: numericAmount,
            description: description.trim(),
            // Removed deposit_date from payload
            is_recurring: isRecurring,
            // Send period only if recurring, otherwise send null to clear it
            recurrence_period: isRecurring ? recurrencePeriod : null,
            // Send end date only if recurring, otherwise send null to clear it
            end_date: isRecurring ? (endDate || null) : null, // Send null if empty string
        };

        try {
            await updateDeposit(depositId, payload);
            setSuccessMessage('Deposit updated successfully!');
            // Optionally navigate back after a delay
            setTimeout(() => {
                onBack();
            }, 1500);
        } catch (err) {
            console.error(`Failed to update deposit:`, err);
            setError(err instanceof Error ? err.message : 'An unknown error occurred.');
        } finally {
            setIsSaving(false);
        }
    };

    // Handler for the "End Now" button
    const handleEndNow = async () => {
        setError(null);
        setSuccessMessage(null);
        setIsEndingNow(true);

        const today = formatDateForInput(new Date()); // Get today's date formatted

        const payload: UpdateDepositPayload = {
            end_date: today,
        };

        try {
            await updateDeposit(depositId, payload);
            setEndDate(today); // Update local state to reflect the change
            setSuccessMessage('Recurring deposit set to end today.');
            // Optionally: Keep the user on the page to see the change
        } catch (err) {
            console.error(`Failed to end deposit now:`, err);
            setError(err instanceof Error ? err.message : 'An unknown error occurred while ending the deposit.');
        } finally {
            setIsEndingNow(false);
        }
    };


    if (isLoading) {
        return <div className="p-4 text-center">Loading deposit details...</div>;
    }

    if (error && !originalDeposit) { // Show critical error if loading failed
        return (
            <div className="p-4">
                 <div className="flex justify-end mb-4">
                    <button onClick={onBack} className="text-sm text-indigo-600 hover:text-indigo-800">&larr; Back</button>
                </div>
                <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative" role="alert">{error}</div>
            </div>
        );
    }

    return (
        <div className="bg-white shadow-md rounded-lg w-full max-w-md">
            <div className="p-4">
                <div className="flex justify-between items-center mb-4">
                    <h1 className="text-2xl font-bold text-gray-700">Edit Deposit</h1>
                    <button onClick={onBack} className="text-sm text-indigo-600 hover:text-indigo-800">&larr; Back</button>
                </div>


                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
                {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

                <form onSubmit={handleSubmit} className="space-y-4">
                    {/* Amount Input */}
                    <div>
                        <label htmlFor="deposit-amount" className="block text-sm font-medium text-gray-700">Amount</label>
                        <input
                            type="number" id="deposit-amount" value={amount}
                            onChange={(e) => setAmount(e.target.value)}
                            placeholder="0.00" step="0.01" min="0.01" required
                            className="mt-1 block w-full input-style"
                        />
                    </div>

                    {/* Description Input */}
                    <div>
                        <label htmlFor="deposit-description" className="block text-sm font-medium text-gray-700">Description</label>
                        <input
                            type="text" id="deposit-description" value={description}
                            onChange={(e) => setDescription(e.target.value)}
                            placeholder="e.g., Salary May, Birthday Gift" required
                            className="mt-1 block w-full input-style"
                        />
                    </div>

                    {/* Deposit Date Display (Static) */}
                    <div>
                        <label className="block text-sm font-medium text-gray-700">
                            {isRecurring ? 'Start Date' : 'Deposit Date'}
                        </label>
                        <p className="mt-1 block w-full input-style bg-gray-100 text-gray-500 cursor-not-allowed">
                            {originalDeposit ? formatDateForInput(originalDeposit.date) : 'N/A'}
                        </p>
                    </div>


                    {/* Recurring Checkbox */}
                    <div className="flex items-start">
                        <div className="flex items-center h-5">
                            <input
                                id="is-recurring" name="is-recurring" type="checkbox"
                                checked={isRecurring}
                                onChange={(e) => setIsRecurring(e.target.checked)}
                                className="focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded"
                            />
                        </div>
                        <div className="ml-3 text-sm">
                            <label htmlFor="is-recurring" className="font-medium text-gray-700">Is this recurring?</label>
                        </div>
                    </div>

                    {/* Recurrence Period Select (Conditional) */}
                    {isRecurring && (
                        <div>
                            <label htmlFor="recurrence-period" className="block text-sm font-medium text-gray-700">Recurrence Period</label>
                            <select
                                id="recurrence-period" value={recurrencePeriod}
                                onChange={(e) => setRecurrencePeriod(e.target.value)}
                                required={isRecurring}
                                className="mt-1 block w-full input-style bg-white"
                            >
                                <option value="weekly">Weekly</option>
                                <option value="monthly">Monthly</option>
                                <option value="yearly">Yearly</option>
                            </select>
                        </div>
                    )}

                    {/* End Date Input (Conditional) */}
                    {isRecurring && (
                        <div>
                            <label htmlFor="end-date" className="block text-sm font-medium text-gray-700">End Date (Optional)</label>
                            <input
                                type="date" id="end-date" value={endDate}
                                onChange={(e) => setEndDate(e.target.value)}
                                className="mt-1 block w-full input-style"
                                // Set min based on original deposit date
                                min={originalDeposit ? formatDateForInput(originalDeposit.date) : undefined}
                            />
                             <p className="text-xs text-gray-500 mt-1">Leave blank for indefinite recurrence.</p>
                             {/* "End Now" Button */}
                             <button
                                type="button"
                                onClick={handleEndNow}
                                disabled={isEndingNow || isSaving || isLoading || (!!endDate && endDate <= formatDateForInput(new Date()))} // Disable if already ended, ending, saving, loading, or end date is today/past
                                className={`mt-2 w-full flex justify-center py-1 px-3 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${
                                    (isEndingNow || isSaving || isLoading || (!!endDate && endDate <= formatDateForInput(new Date())))
                                        ? 'bg-red-300 cursor-not-allowed'
                                        : 'bg-red-500 hover:bg-red-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-red-400'
                                }`}
                            >
                                {isEndingNow ? 'Ending...' : 'End Recurring Deposit Today'}
                            </button>
                        </div>
                    )}


                    {/* Submit Button */}
                    <div className="pt-4">
                        <button
                            type="submit" disabled={isSaving || isLoading || isEndingNow} // Also disable while ending now
                            className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isSaving || isLoading || isEndingNow ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
                        >
                            {isSaving ? 'Saving...' : 'Save Changes'}
                        </button>
                    </div>
                </form>
            </div>
             {/* Simple helper class for inputs */}
             <style>{`
                .input-style {
                    padding: 0.5rem 0.75rem; border: 1px solid #d1d5db; border-radius: 0.375rem; box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05);
                }
                .input-style:focus { outline: none; border-color: #6366f1; box-shadow: 0 0 0 1px #6366f1; }
            `}</style>
        </div>
    );
}

export default EditDepositPage;
