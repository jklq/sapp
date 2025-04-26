import { useState, useEffect, Fragment, useCallback } from 'react'; // Added Fragment, useCallback
import { fetchSpendings, fetchCategories, updateSpendingItem } from './api'; // Added fetchCategories, updateSpendingItem
import { TransactionGroup, SpendingItem, GroupedSpendingsResponse, Category, UpdateSpendingPayload, EditableSharingStatus } from './types'; // Import new types

interface SpendingsListProps {
    onBack: () => void;
}

function SpendingsList({ onBack }: SpendingsListProps) {
    // State now holds an array of TransactionGroup
    // Data states
    const [transactionGroups, setTransactionGroups] = useState<GroupedSpendingsResponse>([]);
    const [categories, setCategories] = useState<Category[]>([]);

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [isFetchingCategories, setIsFetchingCategories] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    const [editError, setEditError] = useState<string | null>(null); // Separate error state for editing

    // Edit state
    const [editingItemId, setEditingItemId] = useState<number | null>(null);
    const [editFormData, setEditFormData] = useState<UpdateSpendingPayload | null>(null);
    const [isSaving, setIsSaving] = useState<boolean>(false);

    // Fetch spendings and categories
    const loadData = useCallback(() => {
        setIsLoading(true);
        setIsFetchingCategories(true);
        setError(null);
        setEditError(null); // Clear edit errors on reload

        const spendingsPromise = fetchSpendings();
        const categoriesPromise = fetchCategories();

        Promise.all([spendingsPromise, categoriesPromise])
            .then(([spendingsData, categoriesData]) => {
                setTransactionGroups(spendingsData);
                setCategories(categoriesData);
            })
            .catch(err => {
                console.error("Failed to load data:", err);
                setError(err instanceof Error ? err.message : 'Failed to load data.');
                // Set empty arrays on error
                setTransactionGroups([]);
                setCategories([]);
            })
            .finally(() => {
                setIsLoading(false);
                setIsFetchingCategories(false);
            });
    }, []); // useCallback with empty dependency array

    useEffect(() => {
        loadData();
    }, [loadData]); // Fetch on mount and when loadData changes (which it won't here)

    // Helper to format date string (can be reused)
    const formatDate = (dateString: string) => {
        try {
            // Use a slightly shorter format for job date maybe?
            return new Date(dateString).toLocaleString(undefined, {
                year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'
            });
        } catch (e) {
            return dateString; // Fallback
        }
    };

    // Helper to format currency
    const formatCurrency = (amount: number) => {
        return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' }); // Adjust currency code if needed
    };

    // --- Edit Handlers ---

    const handleEditClick = (item: SpendingItem) => {
        setEditingItemId(item.id);
        setEditError(null); // Clear previous edit errors

        // Determine initial EditableSharingStatus from the display string
        let initialSharingStatus: EditableSharingStatus = 'Alone'; // Default
        if (item.sharing_status.startsWith('Shared')) {
            initialSharingStatus = 'Shared';
        } else if (item.sharing_status.startsWith('Paid by')) {
            initialSharingStatus = 'Paid by Partner';
        }

        setEditFormData({
            description: item.description || '', // Handle null description
            category_name: item.category_name,
            sharing_status: initialSharingStatus,
        });
    };

    const handleCancelEdit = () => {
        setEditingItemId(null);
        setEditFormData(null);
        setEditError(null);
    };

    const handleEditFormChange = (field: keyof UpdateSpendingPayload, value: string) => {
        if (editFormData) {
            setEditFormData({ ...editFormData, [field]: value });
        }
    };

    const handleSaveEdit = async () => {
        if (!editingItemId || !editFormData) return;

        // Basic validation
        if (!editFormData.category_name) {
            setEditError("Category cannot be empty.");
            return;
        }

        setIsSaving(true);
        setEditError(null);

        try {
            await updateSpendingItem(editingItemId, editFormData);
            handleCancelEdit(); // Close edit form on success
            loadData(); // Refetch data to show changes
        } catch (err) {
            console.error("Failed to save spending item:", err);
            setEditError(err instanceof Error ? err.message : 'Failed to save changes.');
        } finally {
            setIsSaving(false);
        }
    };

    // --- Render Logic ---

    // Helper to render either display row or edit form
    const renderSpendingItemRow = (item: SpendingItem) => {
        if (editingItemId === item.id && editFormData) {
            // Render Edit Form Row
            return (
                <tr key={`${item.id}-edit`} className="bg-yellow-50">
                    {/* Description Input */}
                    <td className="px-4 py-3 whitespace-nowrap">
                        <input
                            type="text"
                            value={editFormData.description}
                            onChange={(e) => handleEditFormChange('description', e.target.value)}
                            className="w-full px-2 py-1 border border-gray-300 rounded text-sm"
                            placeholder="Description"
                        />
                    </td>
                    {/* Category Select */}
                    <td className="px-4 py-3 whitespace-nowrap">
                        <select
                            value={editFormData.category_name}
                            onChange={(e) => handleEditFormChange('category_name', e.target.value)}
                            className="w-full px-2 py-1 border border-gray-300 rounded text-sm bg-white"
                            disabled={isFetchingCategories || categories.length === 0}
                        >
                            {isFetchingCategories ? (
                                <option>Loading...</option>
                            ) : categories.length === 0 ? (
                                <option>No categories</option>
                            ) : (
                                categories.map(cat => (
                                    <option key={cat.id} value={cat.name}>{cat.name}</option>
                                ))
                            )}
                        </select>
                    </td>
                    {/* Amount (Read-only) */}
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500 text-right">
                        {formatCurrency(item.amount)}
                    </td>
                    {/* Sharing Status Select */}
                    <td className="px-4 py-3 whitespace-nowrap">
                         <select
                            value={editFormData.sharing_status}
                            onChange={(e) => handleEditFormChange('sharing_status', e.target.value)}
                            className="w-full px-2 py-1 border border-gray-300 rounded text-sm bg-white"
                        >
                            <option value="Alone">Alone</option>
                            <option value="Shared">Shared</option>
                            <option value="Paid by Partner">Paid by Partner</option>
                        </select>
                    </td>
                    {/* Actions (Save/Cancel) */}
                    <td className="px-4 py-3 whitespace-nowrap text-right text-sm font-medium space-x-2">
                        <button
                            onClick={handleSaveEdit}
                            disabled={isSaving}
                            className={`text-green-600 hover:text-green-900 ${isSaving ? 'opacity-50 cursor-not-allowed' : ''}`}
                        >
                            {isSaving ? 'Saving...' : 'Save'}
                        </button>
                        <button
                            onClick={handleCancelEdit}
                            disabled={isSaving}
                            className="text-gray-600 hover:text-gray-900"
                        >
                            Cancel
                        </button>
                    </td>
                </tr>
            );
        } else {
            // Render Display Row
            return (
                <tr key={item.id}>
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">{item.description || '-'}</td>
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">{item.category_name}</td>
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900 text-right">{formatCurrency(item.amount)}</td>
                    <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                        <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                            item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' :
                            item.sharing_status.startsWith('Shared') ? 'bg-green-100 text-green-800' :
                            item.sharing_status.startsWith('Paid by') ? 'bg-yellow-100 text-yellow-800' :
                            'bg-gray-100 text-gray-800' // Fallback
                        }`}>
                            {item.sharing_status}
                        </span>
                    </td>
                    {/* Action (Edit Button) */}
                    <td className="px-4 py-3 whitespace-nowrap text-right text-sm font-medium">
                        <button
                            onClick={() => handleEditClick(item)}
                            disabled={editingItemId !== null} // Disable other edit buttons while one is active
                            className={`text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                        >
                            Edit
                        </button>
                    </td>
                </tr>
            );
        }
    };


    return (
        <div className="bg-white shadow-md rounded-lg p-6 w-full max-w-4xl"> {/* Increased max-width */}
            <div className="flex justify-between items-center mb-4">
                <h1 className="text-2xl font-bold text-gray-700">Spending History</h1>
                <button
                    onClick={onBack}
                    className="text-sm text-indigo-600 hover:text-indigo-800"
                >
                    &larr; Back to Log Spending
                </button>
            </div>


            {isLoading && <div className="text-center p-4">Loading history...</div>}
            {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error loading history: {error}</div>}
            {editError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error saving changes: {editError}</div>}


            {!isLoading && !error && transactionGroups.length === 0 && (
                <div className="text-center text-gray-500 p-4">No spending history found. Try logging some expenses using the AI mode!</div>
            )}

            {/* Render grouped transactions */}
            {!isLoading && !error && transactionGroups.length > 0 && (
                <div className="space-y-6">
                    {transactionGroups.map((group) => (
                        <div key={group.job_id} className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
                            {/* Transaction Group Header */}
                            <div className="bg-gray-50 p-3 border-b border-gray-200">
                                <div className="flex justify-between items-center flex-wrap gap-2">
                                    <div className="flex-1 min-w-0">
                                        <p className="text-sm font-medium text-indigo-600 truncate" title={group.prompt}>
                                            Prompt: <span className="text-gray-700 font-normal">{group.prompt}</span>
                                        </p>
                                        <p className="text-xs text-gray-500">
                                            {formatDate(group.job_created_at)} by <span className="font-medium">{group.buyer_name}</span> - Total: {formatCurrency(group.total_amount)}
                                        </p>
                                    </div>
                                    {group.is_ambiguity_flagged && (
                                        <div className="flex-shrink-0 ml-4">
                                            <span
                                                className="px-2 py-1 inline-flex text-xs leading-4 font-semibold rounded-full bg-yellow-100 text-yellow-800 cursor-help"
                                                title={`Ambiguity Reason: ${group.ambiguity_flag_reason || 'No reason provided'}`}
                                            >
                                                ⚠️ Ambiguous
                                            </span>
                                        </div>
                                    )}
                                </div>
                            </div>

                            {/* Spending Items Table within the Group */}
                            <div className="overflow-x-auto">
                                <table className="min-w-full divide-y divide-gray-200">
                                    {/* Optional: Add a subtle thead for clarity within the group */}
                                    <thead className="bg-white">
                                        <tr>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Item Desc.</th>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Category</th>
                                            <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Amount</th>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Sharing</th>
                                            <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                                        </tr>
                                    </thead>
                                    <tbody className="bg-white divide-y divide-gray-200">
                                        {group.spendings.map(renderSpendingItemRow)} {/* Use the helper function */}
                                        {group.spendings.length === 0 && (
                                            <tr>
                                                <td colSpan={5} className="px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</td>
                                            </tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

export default SpendingsList;
