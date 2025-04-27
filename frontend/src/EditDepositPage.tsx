import { useState, useEffect, FormEvent, useCallback, useMemo } from 'react';
import { fetchDepositById, updateDeposit } from './api';
import { DepositTemplate, UpdateDepositPayload } from './types';

// Helper to format date to YYYY-MM-DD for input[type=date]
// Ensures that the date is treated as local time when extracting components.
const formatDateForInput = (date: Date | string | null | undefined): string => {
    console.log("[formatDateForInput] Received date input:", date); // Log input
    if (!date) {
        console.log("[formatDateForInput] Input is null or empty, returning empty string.");
        return '';
    }
    try {
        // Attempt to create a Date object. If input is already a Date, this works fine.
        // If input is a string, it tries to parse it. Crucially, Date parsing can be timezone-sensitive.
        // If the backend sends 'YYYY-MM-DD', new Date() might interpret it as UTC midnight.
        // If it sends a full ISO string like '2024-05-15T10:00:00Z', it's UTC.
        // If it sends '2024-05-15T10:00:00', it might be local or UTC depending on browser.
        // For date-only inputs, we generally want the LOCAL date components.
        const d = typeof date === 'string' ? new Date(date) : date;

        // Check if 'd' is a valid Date object after potential parsing
        if (isNaN(d.getTime())) {
            console.error("[formatDateForInput] Invalid Date object after parsing input:", date);
            return '';
        }

        // Use local time components for the YYYY-MM-DD format
        const year = d.getFullYear();
        const month = (d.getMonth() + 1).toString().padStart(2, '0'); // getMonth is 0-indexed
        const day = d.getDate().toString().padStart(2, '0');
        const formatted = `${year}-${month}-${day}`;
        console.log("[formatDateForInput] Successfully formatted:", date, "->", formatted); // Log output
        return formatted;
    } catch (e) {
        console.error("[formatDateForInput] Error during formatting:", date, e);
        return '';
    }
};

// Initial empty state for the form data
const initialFormData: UpdateDepositPayload = {
    amount: 0,
    description: '',
    is_recurring: false,
    recurrence_period: 'monthly',
    end_date: null,
};

interface EditDepositPageProps {
    depositId: number;
    onBack: () => void; // Function to go back to the previous view (e.g., HistoryList)
}

function EditDepositPage({ depositId, onBack }: EditDepositPageProps) {
    // State for the form data, initialized empty
    const [formData, setFormData] = useState<UpdateDepositPayload>(initialFormData);
    // State to store the original deposit date (start date) separately for display and validation
    const [originalDepositDate, setOriginalDepositDate] = useState<string>('');

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(true); // Loading initial data
    const [isSaving, setIsSaving] = useState<boolean>(false); // Saving changes
    const [isEndingNow, setIsEndingNow] = useState<boolean>(false); // State for "End Now" action
    const [error, setError] = useState<string | null>(null);
    const [successMessage, setSuccessMessage] = useState<string | null>(null);

    // Fetch deposit details and populate form
    const loadDeposit = useCallback(() => {
        setIsLoading(true);
        setError(null);
        setSuccessMessage(null); // Clear success message on reload
        fetchDepositById(depositId)
            .then(data => {
                console.log("[EditDepositPage loadDeposit] Received data from API:", JSON.stringify(data)); // Log raw data
                const formattedStartDate = formatDateForInput(data.date);
                const formattedEndDate = formatDateForInput(data.end_date);
                console.log("[EditDepositPage loadDeposit] Formatted start date:", formattedStartDate); // Log formatted date
                console.log("[EditDepositPage loadDeposit] Formatted end date:", formattedEndDate); // Log formatted date

                setOriginalDepositDate(formattedStartDate); // Store original start date

                setFormData({
                    amount: data.amount,
                    description: data.description,
                    is_recurring: data.is_recurring,
                    recurrence_period: data.recurrence_period || 'monthly', // Default if null
                    end_date: formattedEndDate || null, // Store as YYYY-MM-DD or null
                });
            })
            .catch(err => {
                console.error("Failed to load deposit details:", err);
                setError(err instanceof Error ? err.message : 'Failed to load deposit details.');
                setFormData(initialFormData); // Reset form on error
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, [depositId]);

    useEffect(() => {
        loadDeposit();
    }, [loadDeposit]); // Reload when depositId changes (though unlikely in this component structure)

    // Handle changes to form inputs
    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) => {
        const { name, value, type } = e.target;

        setFormData(prev => {
            const newValue = type === 'checkbox' ? (e.target as HTMLInputElement).checked : value;
            const updatedData = { ...prev, [name]: newValue };

            // If toggling recurring off, clear related fields
            if (name === 'is_recurring' && !newValue) {
                updatedData.recurrence_period = null;
                updatedData.end_date = null;
            }
            // If toggling recurring on, ensure period has a default if null/empty
            if (name === 'is_recurring' && newValue && !updatedData.recurrence_period) {
                updatedData.recurrence_period = 'monthly';
            }

            return updatedData;
        });
    };

    // Handle form submission
    const handleSubmit = async (event: FormEvent) => {
        event.preventDefault();
        setError(null);
        setSuccessMessage(null);

        // --- Validation ---
        if (!formData.amount || formData.amount <= 0) {
            setError('Please enter a valid positive amount.');
            return;
        }
        if (!formData.description?.trim()) {
            setError('Please enter a description.');
            return;
        }
        if (formData.is_recurring && !formData.recurrence_period) {
            setError('Please select the recurrence period for recurring deposits.');
            return;
        }
        // Validate end date is not before original deposit date if both are set
        if (formData.end_date && originalDepositDate && formData.end_date < originalDepositDate) {
            setError('End date cannot be before the original deposit start date.');
            return;
        }
        // --- End Validation ---

        setIsSaving(true);

        // Prepare payload - ensure nulls are sent correctly
        const payload: UpdateDepositPayload = {
            amount: formData.amount,
            description: formData.description.trim(),
            is_recurring: formData.is_recurring,
            // Send period only if recurring, otherwise send null to clear it
            recurrence_period: formData.is_recurring ? formData.recurrence_period : null,
            // Send end date only if recurring, otherwise send null to clear it
            // Ensure empty string becomes null
            end_date: formData.is_recurring ? (formData.end_date || null) : null,
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

        // Validate that today is not before the original start date
        if (originalDepositDate && today < originalDepositDate) {
             setError('Cannot end the deposit today as it is before the start date.');
             setIsEndingNow(false);
             return;
        }

        const payload: UpdateDepositPayload = {
            // We only need to send the end_date to update it
            end_date: today,
            // We might need to send is_recurring: true if the backend requires it when setting end_date
            // Check backend logic - assuming just sending end_date is sufficient for update
        };

        try {
            await updateDeposit(depositId, payload);
            // Update local form state to reflect the change immediately
            setFormData(prev => ({ ...prev, end_date: today }));
            setSuccessMessage('Recurring deposit set to end today.');
        } catch (err) {
            console.error(`Failed to end deposit now:`, err);
            setError(err instanceof Error ? err.message : 'An unknown error occurred while ending the deposit.');
        } finally {
            setIsEndingNow(false);
        }
    };

    // Memoize disable state for End Now button
    const isEndNowDisabled = useMemo(() => {
        const today = formatDateForInput(new Date());
        return isEndingNow || isSaving || isLoading || !formData.is_recurring || (!!formData.end_date && formData.end_date <= today) || (!!originalDepositDate && today < originalDepositDate);
    }, [isEndingNow, isSaving, isLoading, formData.is_recurring, formData.end_date, originalDepositDate]);


    // --- Render Logic ---

    if (isLoading) {
        return <div className="p-4 text-center">Loading deposit details...</div>;
    }

    // Show critical error if loading failed and we have no data to display
    if (error && !originalDepositDate) {
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

                {/* Display non-critical loading error */}
                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
                {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

                <form onSubmit={handleSubmit} className="space-y-4">
                    {/* Amount Input */}
                    <div>
                        <label htmlFor="amount" className="block text-sm font-medium text-gray-700">Amount</label>
                        <input
                            type="number" id="amount" name="amount"
                            value={formData.amount}
                            onChange={handleInputChange}
                            placeholder="0.00" step="0.01" min="0.01" required
                            className="mt-1 block w-full input-style"
                        />
                    </div>

                    {/* Description Input */}
                    <div>
                        <label htmlFor="description" className="block text-sm font-medium text-gray-700">Description</label>
                        <input
                            type="text" id="description" name="description"
                            value={formData.description}
                            onChange={handleInputChange}
                            placeholder="e.g., Salary May, Birthday Gift" required
                            className="mt-1 block w-full input-style"
                        />
                    </div>

                    {/* Deposit Date Display (Static) */}
                    <div>
                        <label className="block text-sm font-medium text-gray-700">
                            {formData.is_recurring ? 'Start Date' : 'Deposit Date'} (Cannot be changed)
                        </label>
                        <p className="mt-1 block w-full input-style bg-gray-100 text-gray-500 cursor-not-allowed">
                            {originalDepositDate || 'N/A'}
                        </p>
                    </div>

                    {/* Recurring Checkbox */}
                    <div className="flex items-start">
                        <div className="flex items-center h-5">
                            <input
                                id="is_recurring" name="is_recurring" type="checkbox"
                                checked={formData.is_recurring}
                                onChange={handleInputChange}
                                className="focus:ring-indigo-500 h-4 w-4 text-indigo-600 border-gray-300 rounded"
                            />
                        </div>
                        <div className="ml-3 text-sm">
                            <label htmlFor="is_recurring" className="font-medium text-gray-700">Is this recurring?</label>
                        </div>
                    </div>

                    {/* Recurrence Period Select (Conditional) */}
                    {formData.is_recurring && (
                        <div>
                            <label htmlFor="recurrence_period" className="block text-sm font-medium text-gray-700">Recurrence Period</label>
                            <select
                                id="recurrence_period" name="recurrence_period"
                                value={formData.recurrence_period || 'monthly'} // Ensure value is controlled
                                onChange={handleInputChange}
                                required={formData.is_recurring}
                                className="mt-1 block w-full input-style bg-white"
                            >
                                <option value="weekly">Weekly</option>
                                <option value="monthly">Monthly</option>
                                <option value="yearly">Yearly</option>
                            </select>
                        </div>
                    )}

                    {/* End Date Input (Conditional) */}
                    {formData.is_recurring && (
                        <div>
                            <label htmlFor="end_date" className="block text-sm font-medium text-gray-700">End Date (Optional)</label>
                            <input
                                type="date" id="end_date" name="end_date"
                                value={formData.end_date || ''} // Use empty string if null for input value
                                onChange={handleInputChange}
                                className="mt-1 block w-full input-style"
                                min={originalDepositDate || undefined} // Set min based on original start date
                            />
                            <p className="text-xs text-gray-500 mt-1">Leave blank for indefinite recurrence.</p>
                            {/* "End Now" Button */}
                            <button
                                type="button"
                                onClick={handleEndNow}
                                disabled={isEndNowDisabled}
                                className={`mt-2 w-full flex justify-center py-1 px-3 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isEndNowDisabled
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
                            type="submit" disabled={isSaving || isLoading || isEndingNow}
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
                .input-style.bg-gray-100 { background-color: #f3f4f6; } /* Ensure disabled style applies */
            `}</style>
        </div>
    );
}

export default EditDepositPage;
