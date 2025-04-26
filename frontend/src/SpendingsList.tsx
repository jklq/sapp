import { useState, useEffect, useCallback } from 'react'; // Removed Fragment
import { fetchSpendings, fetchCategories, updateSpendingItem } from './api'; // Added fetchCategories, updateSpendingItem
import { SpendingItem, GroupedSpendingsResponse, Category, UpdateSpendingPayload, EditableSharingStatus } from './types'; // Removed TransactionGroup, Import new types

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
    const [deletingJobId, setDeletingJobId] = useState<number | null>(null); // Track which job is being deleted
    const [deleteError, setDeleteError] = useState<string | null>(null); // Separate error state for deletion

    // Expansion state
    const [expandedGroupIds, setExpandedGroupIds] = useState<Set<number>>(new Set());

    // Fetch spendings and categories
    const loadData = useCallback(() => {
        setIsLoading(true);
        setIsFetchingCategories(true);
        setError(null);
        setEditError(null); // Clear edit errors on reload
        setDeleteError(null); // Clear delete errors on reload

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

    // Toggle group expansion
    const toggleGroupExpansion = (jobId: number) => {
        setExpandedGroupIds(prev => {
            const newSet = new Set(prev);
            if (newSet.has(jobId)) {
                newSet.delete(jobId);
            } else {
                newSet.add(jobId);
            }
            return newSet;
        });
    };

    // --- Delete Handler ---
    const handleDeleteJob = async (jobId: number) => {
        // Basic confirmation dialog
        if (!window.confirm(`Are you sure you want to delete this entire transaction (Job ID: ${jobId}) and all its associated spending items? This action cannot be undone.`)) {
            return;
        }

        setDeletingJobId(jobId);
        setDeleteError(null);

        try {
            await deleteAIJob(jobId);
            // Option 1: Refetch all data
            // loadData();
            // Option 2: Remove the group from local state for faster UI update
            setTransactionGroups(prevGroups => prevGroups.filter(group => group.job_id !== jobId));
        } catch (err) {
            console.error("Failed to delete job:", err);
            setDeleteError(err instanceof Error ? err.message : 'Failed to delete the transaction.');
        } finally {
            setDeletingJobId(null);
        }
    };


    // --- Render Logic ---

    // Helper to render either display row or edit form - now responsive
    const renderSpendingItemRow = (item: SpendingItem) => {
        const isEditing = editingItemId === item.id && editFormData;

        // Common classes for the container (card on mobile, table row on md+)
        const containerClasses = `block md:table-row ${isEditing ? 'bg-yellow-50' : 'bg-white'} border-b border-gray-200 md:border-none`; // Add border for mobile cards

        if (isEditing) {
            // --- Render Edit Form (Responsive) ---
            return (
                <div key={`${item.id}-edit`} className={containerClasses}>
                    {/* Description Input (Full width on mobile, table cell on md+) */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap">
                         <label htmlFor={`edit-desc-${item.id}`} className="text-xs font-medium text-gray-500 uppercase md:hidden">Desc.</label>
                         <input
                            id={`edit-desc-${item.id}`}
                            type="text"
                            value={editFormData.description}
                            onChange={(e) => handleEditFormChange('description', e.target.value)}
                            className="mt-1 md:mt-0 w-full px-2 py-1 border border-gray-300 rounded text-sm"
                            placeholder="Description"
                        />
                    </div>
                    {/* Category Select (Full width on mobile, table cell on md+) */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap">
                         <label htmlFor={`edit-cat-${item.id}`} className="text-xs font-medium text-gray-500 uppercase md:hidden">Category</label>
                         <select
                            id={`edit-cat-${item.id}`}
                            value={editFormData.category_name}
                            onChange={(e) => handleEditFormChange('category_name', e.target.value)}
                            className="mt-1 md:mt-0 w-full px-2 py-1 border border-gray-300 rounded text-sm bg-white"
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
                    </div>
                    {/* Amount (Read-only) - Moved and aligned left on mobile */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap text-sm text-gray-500 md:text-right"> {/* Keep text-right for md+ */}
                         <span className="text-xs font-medium text-gray-500 uppercase md:hidden">Amount: </span>
                         {formatCurrency(item.amount)}
                    </div>
                    {/* Sharing Status Select (Full width on mobile, table cell on md+) */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap">
                         <label htmlFor={`edit-share-${item.id}`} className="text-xs font-medium text-gray-500 uppercase md:hidden">Sharing</label>
                         <select
                            id={`edit-share-${item.id}`}
                            value={editFormData.sharing_status}
                            onChange={(e) => handleEditFormChange('sharing_status', e.target.value)}
                            className="mt-1 md:mt-0 w-full px-2 py-1 border border-gray-300 rounded text-sm bg-white"
                        >
                            <option value="Alone">Alone</option>
                            <option value="Shared">Shared</option>
                            <option value="Paid by Partner">Paid by Partner</option>
                        </select>
                    </div>
                    {/* Actions (Save/Cancel) - Moved below other fields on mobile */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap text-right text-sm font-medium space-x-2">
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
                    </div>
                </div> // Close the main div for the edit row
            );
        } else {
            // --- Render Display Row/Card (Responsive) ---
            return (
                // Container for one item: block on mobile, table-row on md+
                // Add padding and border for mobile card appearance
                <div key={item.id} className={`${containerClasses} p-3 md:p-0 md:border-b md:border-gray-200`}>

                    {/* Mobile View Structure (md:hidden) */}
                    <div className="md:hidden space-y-2">
                        {/* Description (Primary info) */}
                        <div>
                            <span className="text-xs font-medium text-gray-500 uppercase">Description</span>
                            <p className="text-sm text-gray-900 break-words">{item.description || '-'}</p>
                        </div>

                        {/* Category */}
                        <div>
                            <span className="text-xs font-medium text-gray-500 uppercase">Category</span>
                            <p className="text-sm text-gray-500">{item.category_name}</p>
                        </div>

                        {/* Amount */}
                        <div>
                            <span className="text-xs font-medium text-gray-500 uppercase">Amount</span>
                            <p className="text-sm text-gray-900">{formatCurrency(item.amount)}</p>
                        </div>

                        {/* Sharing Status */}
                        <div>
                            <span className="text-xs font-medium text-gray-500 uppercase">Sharing</span>
                            <div> {/* Wrap badge in div for block layout */}
                                <span className={`mt-1 px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                                    item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' :
                                    item.sharing_status.startsWith('Shared') ? 'bg-green-100 text-green-800' :
                                    item.sharing_status.startsWith('Paid by') ? 'bg-yellow-100 text-yellow-800' :
                                    'bg-gray-100 text-gray-800' // Fallback
                                }`}>
                                    {item.sharing_status}
                                </span>
                            </div>
                        </div>

                        {/* Action (Edit Button) */}
                        <div className="pt-2 text-right"> {/* Add padding top for separation */}
                            <button
                                onClick={() => handleEditClick(item)}
                                disabled={editingItemId !== null} // Disable other edit buttons while one is active
                                className={`text-sm text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                            >
                                Edit
                            </button>
                        </div>
                    </div>

                    {/* Desktop Table Cell View (hidden on mobile) */}
                    {/* Description */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-sm text-gray-900">{item.description || '-'}</div>
                    {/* Category */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-sm text-gray-500">{item.category_name}</div>
                    {/* Amount */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-sm text-gray-900 text-right">{formatCurrency(item.amount)}</div>
                    {/* Sharing Status */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-sm text-gray-500">
                        <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                            item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' :
                            item.sharing_status.startsWith('Shared') ? 'bg-green-100 text-green-800' :
                            item.sharing_status.startsWith('Paid by') ? 'bg-yellow-100 text-yellow-800' :
                            'bg-gray-100 text-gray-800' // Fallback
                        }`}>
                            {item.sharing_status}
                        </span>
                    </div>
                    {/* Action (Edit Button) */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-right text-sm font-medium">
                        <button
                            onClick={() => handleEditClick(item)}
                            disabled={editingItemId !== null}
                            className={`text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                        >
                            Edit
                        </button>
                    </div>
                </div> // Close the main div for the display row/card
            );
        }
    };


    return (
        // Remove p-6, add p-4 inside
        <div className="bg-white shadow-md rounded-lg w-full max-w-4xl">
            <div className="p-4"> {/* Add inner padding */}
                <div className="flex flex-wrap justify-between items-center mb-4 gap-2"> {/* Allow wrapping */}
                    <h1 className="text-2xl font-bold text-gray-700">Spending History</h1>
                    <button
                    onClick={onBack}
                    className="text-sm text-indigo-600 hover:text-indigo-800"
                >
                    &larr; Back to Log Spending
                </button>
            </div>


            {isLoading && <div className="text-center p-4">Loading history...</div>}
            {/* Display general loading/fetch error */}
            {error && !isLoading && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error loading history: {error}</div>}
            {/* Display edit error */}
            {editError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error saving changes: {editError}</div>}
            {/* Display delete error */}
            {deleteError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error deleting transaction: {deleteError}</div>}


            {!isLoading && !error && transactionGroups.length === 0 && (
                <div className="text-center text-gray-500 p-4">No spending history found. Try logging some expenses using the AI mode!</div>
            )}

            {/* Render grouped transactions */}
            {!isLoading && !error && transactionGroups.length > 0 && (
                <div className="space-y-6">
                    {transactionGroups.map((group) => (
                        <div key={group.job_id} className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
                            {/* Transaction Group Header - Make clickable */}
                            <div
                                className="bg-gray-50 p-3 border-b border-gray-200 cursor-pointer hover:bg-gray-100"
                                onClick={() => toggleGroupExpansion(group.job_id)}
                            >
                                <div className="flex justify-between items-center flex-wrap gap-2"> {/* Use items-center */}
                                    {/* Left side: Prompt, Date, Buyer, Total */}
                                    <div className="flex-1 min-w-0 mr-2"> {/* Allow shrinking, add margin */}
                                        <p className="text-sm font-medium text-indigo-600 break-words" title={group.prompt}>
                                            Prompt: <span className="text-gray-700 font-normal">{group.prompt}</span>
                                        </p>
                                        <p className="text-xs text-gray-500">
                                            {formatDate(group.job_created_at)} by <span className="font-medium">{group.buyer_name}</span> - Total: {formatCurrency(group.total_amount)}
                                        </p>
                                    </div>
                                    {/* Right side: Ambiguity flag, Delete Button, Expander Icon */}
                                    <div className="flex items-center flex-shrink-0 space-x-2"> {/* Add space-x-2 */}
                                        {group.is_ambiguity_flagged && (
                                            <span
                                                className="px-2 py-1 inline-flex text-xs leading-4 font-semibold rounded-full bg-yellow-100 text-yellow-800 cursor-help"
                                                title={`Ambiguity Reason: ${group.ambiguity_flag_reason || 'No reason provided'}`}
                                                onClick={(e) => e.stopPropagation()} // Prevent title click from toggling group
                                            >
                                                ⚠️ Ambiguous
                                            </span>
                                        )}
                                        {/* Delete Button */}
                                        <button
                                            onClick={(e) => {
                                                e.stopPropagation(); // Prevent toggling group when clicking delete
                                                handleDeleteJob(group.job_id);
                                            }}
                                            disabled={deletingJobId === group.job_id || editingItemId !== null} // Disable while deleting this or editing any item
                                            className={`text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1`}
                                            title="Delete this entire transaction"
                                        >
                                            {/* Simple Trash Icon (SVG or character) */}
                                            <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                            </svg>
                                            {/* Show spinner if deleting this specific job */}
                                            {deletingJobId === group.job_id && <span className="ml-1 text-xs">(Deleting...)</span>}
                                        </button>
                                        {/* Expander Icon */}
                                        <span className="text-gray-500 text-lg cursor-pointer"> {/* Make icon itself clickable */}
                                            {expandedGroupIds.has(group.job_id) ? '▲' : '▼'}
                                        </span>
                                    </div>
                                </div>
                            </div>

                            {/* Spending Items Container (Conditional Rendering) */}
                            {expandedGroupIds.has(group.job_id) && (
                                <div className="bg-white"> {/* Add bg-white for contrast */}
                                    {/* Table structure for medium screens and up */}
                                    <table className="min-w-full hidden md:table">
                                    {/* Table Head (Hidden on mobile) */}
                                    <thead className="bg-white hidden md:table-header-group">
                                        <tr>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Item Desc.</th>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Category</th>
                                            <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Amount</th>
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Sharing</th>
                                            <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                                        </tr>
                                    </thead>
                                    {/* Table Body (Hidden on mobile, rendered via helper) */}
                                    <tbody className="hidden md:table-row-group">
                                        {group.spendings.map(renderSpendingItemRow)}
                                        {group.spendings.length === 0 && (
                                            <tr className="md:table-row">
                                                <td colSpan={5} className="md:table-cell px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</td>
                                            </tr>
                                        )}
                                    </tbody>
                                </table>
                                {/* Card/List structure for mobile (rendered via helper) */}
                                {/* Use space-y for separation instead of divide-y for more control */}
                                <div className="md:hidden space-y-3 p-2 bg-gray-50"> {/* Add padding and slight background */}
                                     {group.spendings.map(renderSpendingItemRow)}
                                     {group.spendings.length === 0 && (
                                        <div className="px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</div>
                                     )}
                                </div>
                            </div>
                            )} {/* End conditional rendering for expanded group */}
                        </div>
                    ))}
                </div>
            )}
            </div> {/* Close inner padding div */}
        </div>
    );
}

export default SpendingsList;
