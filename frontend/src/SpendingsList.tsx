import { useState, useEffect, useCallback } from 'react'; // Removed useMemo
import { fetchHistory, fetchCategories, updateSpendingItem, deleteAIJob } from './api'; // Use fetchHistory
// Import HistoryListItem from types
// Removed unused TransactionGroup import again
import { SpendingItem, Category, UpdateSpendingPayload, EditableSharingStatus, HistoryResponse, DepositItem, HistoryListItem } from './types';

interface HistoryListProps { // Renamed props interface
    onBack: () => void;
    loggedInUserName: string | null; // Add missing prop
}

// Removed unused local HistoryListItem type definition

function HistoryList({ onBack, loggedInUserName }: HistoryListProps) { // Renamed component, added prop destructuring
    // Data states
    // Renamed historyData to historyResponse for clarity
    const [historyResponse, setHistoryResponse] = useState<HistoryResponse | null>(null); // Store the raw response
    const [categories, setCategories] = useState<Category[]>([]); // Still needed for editing spendings

    // UI states
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [isFetchingCategories, setIsFetchingCategories] = useState<boolean>(true); // Still needed for editing spendings
    const [error, setError] = useState<string | null>(null);
    const [editError, setEditError] = useState<string | null>(null); // Separate error state for editing

    // Edit state
    const [editingItemId, setEditingItemId] = useState<number | null>(null);
    const [editFormData, setEditFormData] = useState<UpdateSpendingPayload | null>(null);
    const [isSaving, setIsSaving] = useState<boolean>(false);
    const [deletingJobId, setDeletingJobId] = useState<number | null>(null); // Track which job is being deleted
    const [deleteError, setDeleteError] = useState<string | null>(null); // Separate error state for deletion

    // Expansion state
    const [expandedGroupIds, setExpandedGroupIds] = useState<Set<number>>(new Set()); // Keep for spending groups

    // Fetch history and categories
    const loadData = useCallback(() => {
        setIsLoading(true);
        setIsFetchingCategories(true); // Fetch categories for editing spendings
        setError(null);
        setEditError(null);
        setDeleteError(null);

        const historyPromise = fetchHistory(); // Use fetchHistory
        const categoriesPromise = fetchCategories(); // Still fetch categories

        Promise.all([historyPromise, categoriesPromise])
            .then(([fetchedHistoryResponse, categoriesData]) => { // Use different name to avoid conflict
                setHistoryResponse(fetchedHistoryResponse); // Store the combined history response
                setCategories(categoriesData); // Store categories
            })
            .catch(err => {
                console.error("Failed to load history or categories:", err);
                setError(err instanceof Error ? err.message : 'Failed to load history or categories.');
                // Set null/empty on error
                setHistoryResponse(null); // Use the renamed state setter
                setCategories([]); // Ensure categories is empty on error
            })
            .finally(() => {
                setIsLoading(false);
                setIsFetchingCategories(false);
            });
    }, []); // useCallback with empty dependency array

    useEffect(() => {
        loadData();
    }, [loadData]);

    // Helper to format date string (can be reused)
    const formatDate = (dateString: string | Date, options?: Intl.DateTimeFormatOptions) => {
        try {
            const defaultOptions: Intl.DateTimeFormatOptions = {
                year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'
            };
            const finalOptions = { ...defaultOptions, ...options };
            const date = typeof dateString === 'string' ? new Date(dateString) : dateString;
            return date.toLocaleString(undefined, finalOptions);
        } catch (e) {
            return String(dateString); // Fallback
        }
    };

    // Helper to format currency
    const formatCurrency = (amount: number) => {
        return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' }); // Adjust currency code if needed
    };

    // --- Edit Handlers ---

    const handleEditClick = (item: SpendingItem, buyerName: string | null | undefined) => {
        // Prevent editing if the logged-in user is not the buyer
        if (buyerName !== loggedInUserName) {
            console.warn("Attempted to edit an item bought by the partner.");
            setEditError("You can only edit items you paid for."); // Set an error message
            return;
        }

        setEditingItemId(item.id);
        setEditError(null); // Clear previous edit errors

        // Determine initial EditableSharingStatus from the display string
        let initialSharingStatus: EditableSharingStatus = 'Alone'; // Default
        // Adjust logic based on the backend's perspective strings
        if (item.sharing_status.startsWith('Shared with')) { // "Shared with Partner" or "Shared with You" -> Shared
            initialSharingStatus = 'Shared';
        } else if (item.sharing_status.startsWith('Paid by Partner')) { // "Paid by Partner" -> Paid by Partner
            initialSharingStatus = 'Paid by Partner';
        } else if (item.sharing_status.startsWith('Paid by You')) { // "Paid by You" -> Paid by Partner (from user's edit perspective)
             initialSharingStatus = 'Paid by Partner';
        } else if (item.sharing_status === 'Alone') { // "Alone" -> Alone
             initialSharingStatus = 'Alone';
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

    // Toggle spending group expansion
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
            loadData();
            // Option 2: Remove the group from local state (more complex with combined list)
            // setHistoryData(prevData => {
            //     if (!prevData) return null;
            //     return {
            //         ...prevData,
            //         spending_groups: prevData.spending_groups.filter(group => group.job_id !== jobId)
            //     };
            // });
        } catch (err) {
            console.error("Failed to delete spending group job:", err);
            setDeleteError(err instanceof Error ? err.message : 'Failed to delete the spending group.');
        } finally {
            setDeletingJobId(null);
        }
    };

    // --- Render Logic ---
    // NOTE: sortedHistoryItems useMemo hook removed as historyItems state is already sorted and flat.

    // Helper to render a deposit item
    const renderDepositItem = (item: DepositItem) => {
        return (
            <div key={`dep-${item.id}`} className="border border-green-200 bg-green-50 rounded-lg shadow-sm overflow-hidden p-3">
                 <div className="flex justify-between items-center flex-wrap gap-2">
                    {/* Left side: Icon, Description, Date */}
                    <div className="flex items-center space-x-3 flex-1 min-w-0 mr-2">
                         {/* Deposit Icon */}
                         <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6 text-green-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                            <path strokeLinecap="round" strokeLinejoin="round" d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5l-3 3m0 0l-3-3m3 3V8" />
                        </svg>
                        <div>
                            <p className="text-sm font-medium text-green-800 break-words">
                                Deposit: <span className="text-gray-700 font-normal">{item.description}</span>
                            </p>
                            <p className="text-xs text-gray-500">
                                {formatDate(item.date, { hour: undefined, minute: undefined })} {/* Show only date */}
                                {item.is_recurring && <span className="ml-2 text-xs italic">({item.recurrence_period || 'Recurring'})</span>}
                            </p>
                        </div>
                    </div>
                    {/* Right side: Amount */}
                    <div className="flex-shrink-0">
                        <p className="text-lg font-semibold text-green-700">
                            +{formatCurrency(item.amount)}
                        </p>
                    </div>
                 </div>
            </div>
        );
    };

    // Helper to render either display row or edit form - now responsive
    // Added partnerNameFromGroup argument
    const renderSpendingItemRow = (item: SpendingItem, partnerNameFromGroup: string | null | undefined) => {
        const isEditing = editingItemId === item.id && editFormData;

        // Common classes for the container (card on mobile, table row on md+)
        const containerClasses = `block md:table-row ${isEditing ? 'bg-yellow-50' : 'bg-white'} border-b border-gray-200 md:border-none`; 

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
                            {/* Dynamically display partner name if available */}
                            <option value="Alone">Alone</option>
                            <option value="Shared">Shared {item.partner_name ? `with ${item.partner_name}` : ''}</option>
                            <option value="Paid by Partner">Paid by {item.partner_name || 'Partner'}</option>
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
                            <div>
                                <span className={`mt-1 px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                                    item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' : // User paid, alone
                                    item.sharing_status.startsWith('Shared with Partner') ? 'bg-green-100 text-green-800' : // User paid, shared
                                    item.sharing_status.startsWith('Paid by Partner') ? 'bg-yellow-100 text-yellow-800' : // User paid, partner pays
                                    item.sharing_status.startsWith('Shared with You') ? 'bg-teal-100 text-teal-800' : // Partner paid, shared
                                    item.sharing_status.startsWith('Paid by You') ? 'bg-orange-100 text-orange-800' : // Partner paid, user pays
                                    item.sharing_status.includes('Alone') ? 'bg-gray-100 text-gray-800' : // Partner paid, alone
                                    'bg-gray-100 text-gray-800' // Fallback
                                }`}>
                                    {item.sharing_status}
                                </span>
                            </div>
                        </div>

                        {/* Action (Edit Button) - Conditionally render/disable */}
                        {partnerNameFromGroup === loggedInUserName && ( // Only show if current user is the buyer
                            <div className="pt-2 text-right">
                                <button
                                    onClick={() => handleEditClick(item, partnerNameFromGroup)}
                                    disabled={editingItemId !== null} // Disable other edit buttons while one is active
                                    className={`text-sm text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                                >
                                    Edit
                                </button>
                            </div>
                        )}
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
                            item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' : // User paid, alone
                            item.sharing_status.startsWith('Shared with Partner') ? 'bg-green-100 text-green-800' : // User paid, shared
                            item.sharing_status.startsWith('Paid by Partner') ? 'bg-yellow-100 text-yellow-800' : // User paid, partner pays
                            item.sharing_status.startsWith('Shared with You') ? 'bg-teal-100 text-teal-800' : // Partner paid, shared
                            item.sharing_status.startsWith('Paid by You') ? 'bg-orange-100 text-orange-800' : // Partner paid, user pays
                            item.sharing_status.includes('Alone') ? 'bg-gray-100 text-gray-800' : // Partner paid, alone
                            'bg-gray-100 text-gray-800' // Fallback
                        }`}>
                            {item.sharing_status}
                        </span>
                    </div>
                    {/* Action (Edit Button) - Conditionally render/disable */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-right text-sm font-medium">
                         {partnerNameFromGroup === loggedInUserName ? ( // Only enable if current user is the buyer
                            <button
                                onClick={() => handleEditClick(item, partnerNameFromGroup)}
                                disabled={editingItemId !== null} // Disable if any item is being edited
                                className={`text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                            >
                                Edit
                            </button>
                         ) : (
                            <span className="text-gray-400 text-xs italic"></span> // Indicate non-editable
                         )}
                    </div>
                </div>
            );
        }
    };


    return (
        // Remove p-6, add p-4 inside
        <div className="bg-white shadow-md rounded-lg w-full max-w-4xl">
            <div className="p-4"> {/* Add inner padding */}
                <div className="flex flex-wrap justify-between items-center mb-4 gap-2"> {/* Allow wrapping */}
                    <h1 className="text-2xl font-bold text-gray-700">History</h1> {/* Renamed title */}
                    <button
                    onClick={onBack}
                    className="text-sm text-indigo-600 hover:text-indigo-800"
                >
                    &larr; Back {/* Simplified back button text */}
                </button>
            </div>


            {isLoading && <div className="text-center p-4">Loading history...</div>}
            {/* Display general loading/fetch error */}
            {error && !isLoading && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error loading history: {error}</div>}
            {/* Display edit error */}
            {editError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error saving changes: {editError}</div>}
            {/* Display delete error */}
            {deleteError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error deleting spending group: {deleteError}</div>}


            {/* Use historyResponse.history */}
            {!isLoading && !error && (!historyResponse || historyResponse.history.length === 0) && (
                <div className="text-center text-gray-500 p-4">No history found. Try logging some expenses or deposits!</div>
            )}

            {/* Render combined history items from the flat list */}
            {/* Use historyResponse.history */}
            {!isLoading && !error && historyResponse && historyResponse.history.length > 0 && (
                <div className="space-y-4">
                    {/* Add type HistoryListItem to item */}
                    {historyResponse.history.map((item: HistoryListItem) => {
                        // Removed unused key variable declaration

                        if (item.type === 'deposit') {
                            // Render Deposit Item - Cast to DepositItem for type safety
                            // Key is applied in renderDepositItem
                            return renderDepositItem(item as DepositItem);
                        } else {
                            // Render Spending Group (TransactionGroup)
                            const group = item; // item is a TransactionGroup here
                            return (
                                <div key={`sg-${group.job_id}`} className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
                                    {/* Transaction Group Header - Make clickable */}
                                    <div
                                        className="bg-gray-50 p-3 border-b border-gray-200 cursor-pointer hover:bg-gray-100"
                                        onClick={() => toggleGroupExpansion(group.job_id)}
                                    >
                                        <div className="flex justify-between items-center flex-wrap gap-2">
                                            {/* Left side: Prompt, Date, Buyer, Total */}
                                            <div className="flex items-center space-x-3 flex-1 min-w-0 mr-2">
                                                {/* Spending Icon */}
                                                <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6 text-red-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                                                  <path strokeLinecap="round" strokeLinejoin="round" d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
                                                </svg>
                                                <div>
                                                    <p className="text-sm font-medium text-indigo-600 break-words" title={group.prompt}>
                                                        Spending: <span className="text-gray-700 font-normal">{group.prompt}</span>
                                                    </p>
                                                    <p className="text-xs text-gray-500">
                                                        {formatDate(group.date)} by <span className="font-medium">{group.buyer_name}</span> - Total: <span className="font-semibold text-red-700">{formatCurrency(group.total_amount)}</span>
                                                    </p>
                                                </div>
                                            </div>
                                            {/* Right side: Ambiguity flag, Delete Button, Expander Icon */}
                                            <div className="flex items-center flex-shrink-0 space-x-2">
                                                {group.is_ambiguity_flagged && (
                                                    <span
                                                        className="px-2 py-1 inline-flex text-xs leading-4 font-semibold rounded-full bg-yellow-100 text-yellow-800 cursor-help"
                                                        title={`Ambiguity Reason: ${group.ambiguity_flag_reason || 'No reason provided'}`}
                                                        onClick={(e) => e.stopPropagation()}
                                                    >
                                                        ⚠️ Ambiguous
                                                    </span>
                                                )}
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDeleteJob(group.job_id); }}
                                                    disabled={deletingJobId === group.job_id || editingItemId !== null}
                                                    className={`text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1`}
                                                    title="Delete this entire spending group"
                                                >
                                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                                    </svg>
                                                    {deletingJobId === group.job_id && <span className="ml-1 text-xs">(Deleting...)</span>}
                                                </button>
                                                <span className="text-gray-500 text-lg cursor-pointer">
                                                    {expandedGroupIds.has(group.job_id) ? '▲' : '▼'}
                                                </span>
                                            </div>
                                        </div>
                                    </div>

                                    {/* Spending Items Container (Conditional Rendering) */}
                                    {expandedGroupIds.has(group.job_id) && (
                                        <div className="bg-white">
                                            <table className="min-w-full hidden md:table">
                                                <thead className="bg-white hidden md:table-header-group">
                                                    <tr>
                                                        <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Item Desc.</th>
                                                        <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Category</th>
                                                        <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Amount</th>
                                                        <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Sharing</th>
                                                        <th scope="col" className="px-4 py-2 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Actions</th>
                                                    </tr>
                                                </thead>
                                                <tbody className="hidden md:table-row-group">
                                                    {/* Extract partner name and pass to renderer */}
                                                    {( () => { // Wrap IIFE in parentheses
                                                        const partnerNameFromGroup = group.spendings.length > 0 ? group.spendings[0].partner_name : null;
                                                        return group.spendings.map((spendingItem: SpendingItem) => renderSpendingItemRow(spendingItem, partnerNameFromGroup)); // Add type annotation
                                                    })() }
                                                    {group.spendings.length === 0 && (
                                                        <tr className="md:table-row">
                                                            <td colSpan={5} className="md:table-cell px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</td>
                                                        </tr>
                                                    )}
                                                </tbody>
                                            </table>
                                            <div className="md:hidden space-y-3 p-2 bg-gray-50">
                                                {/* Extract partner name and pass to renderer */}
                                                {( () => { // Wrap IIFE in parentheses
                                                    const partnerNameFromGroup = group.spendings.length > 0 ? group.spendings[0].partner_name : null;
                                                    return group.spendings.map((spendingItem: SpendingItem) => renderSpendingItemRow(spendingItem, partnerNameFromGroup)); // Add type annotation
                                                })() }
                                                {group.spendings.length === 0 && (
                                                    <div className="px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</div>
                                                )}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            );
                        }
                    })}
                </div>
            )}
            </div> {/* Close inner padding div */}
        </div>
    );
}

export default HistoryList; // Renamed export
