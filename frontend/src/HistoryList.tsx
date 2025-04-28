import { useState, useEffect, useCallback, useMemo } from 'react';
import { fetchHistory, fetchCategories, updateSpendingItem, deleteAIJob, deleteDeposit } from './api';
// Removed unused TransactionGroup import
import { Category, UpdateSpendingPayload, EditableSharingStatus, HistoryListItem, SpendingItem, DepositItem } from './types';

interface HistoryListProps {
    onBack: () => void;
    
    onNavigateToEditDeposit: (depositId: number) => void;
}


function HistoryList({ onBack, onNavigateToEditDeposit }: HistoryListProps) {
    // Data states
    const [historyItems, setHistoryItems] = useState<HistoryListItem[]>([]); // Store the flat list
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
    const [deletingDepositId, setDeletingDepositId] = useState<number | null>(null); // Track which deposit is being deleted
    const [deleteError, setDeleteError] = useState<string | null>(null); // Combined error state for deletion

    // Expansion state
    const [expandedGroupIds, setExpandedGroupIds] = useState<Set<number>>(new Set()); // Keep for spending groups
    const [expandedDepositKeys, setExpandedDepositKeys] = useState<Set<string>>(new Set()); // State for expanded deposits


    // Fetch history and categories
    const loadData = useCallback(() => {
        setIsLoading(true);
        setIsFetchingCategories(true); // Fetch categories for editing spendings
        setError(null);
        setEditError(null);
        setDeleteError(null);

        const historyPromise = fetchHistory();
        const categoriesPromise = fetchCategories(); // Still fetch categories

        Promise.all([historyPromise, categoriesPromise])
            .then(([historyResponse, categoriesData]) => {
                // The response now has a flat 'history' array
                setHistoryItems(historyResponse.history || []); // Store the flat list
                setCategories(categoriesData); // Store categories
            })
            .catch(err => {
                console.error("Failed to load history or categories:", err);
                setError(err instanceof Error ? err.message : 'Failed to load history or categories.');
                // Set null/empty on error
                setHistoryItems([]); // Ensure historyItems is empty on error
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

    // Helper to format date string
    const formatDate = (dateString: string | Date, options?: Intl.DateTimeFormatOptions) => {
        try {
            const defaultOptions: Intl.DateTimeFormatOptions = {
                year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit'
            };
            const finalOptions = { ...defaultOptions, ...options };
            const date = typeof dateString === 'string' ? new Date(dateString) : dateString;
            return date.toLocaleString(undefined, finalOptions);
        } catch (e) {
            return String(dateString);
        }
    };

    // Helper to format currency
    const formatCurrency = (amount: number) => {
        // Adjust currency code if needed
        return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' });
    };

    // Calculate total balance
    const totalBalance = useMemo(() => {
        return historyItems.reduce((acc, item) => {
            if (item.type === 'deposit') {
                // Ensure amount is treated as number, default to 0 if undefined/null
                return acc + (Number(item.amount) || 0);
            } else if (item.type === 'spending_group') {
                // Ensure total_amount is treated as number, default to 0 if undefined/null
                return acc - (Number(item.total_amount) || 0);
            }
            return acc;
        }, 0);
    }, [historyItems]); // Recalculate only when historyItems changes

    // --- Edit Handlers ---

    const handleEditClick = (item: SpendingItem) => {
        setEditingItemId(item.id);
        setEditError(null); // Clear previous edit errors

        // Determine initial EditableSharingStatus from the display string
        let initialSharingStatus: EditableSharingStatus = 'Alone';
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

    // --- Deposit Action Handlers ---

    const handleEditDepositClick = (depositId: number) => {
        // Call the callback passed from App.tsx to handle navigation
        onNavigateToEditDeposit(depositId);
    };

    const handleDeleteDepositClick = async (depositId: number, isRecurring: boolean) => {
        let confirmMessage = "Are you sure you want to delete this deposit?";
        if (isRecurring) {
            confirmMessage = "This is a recurring deposit template. Deleting it will remove the template and prevent future occurrences. Existing past occurrences in this list will remain visible until the next refresh. Are you sure you want to delete the template?";
        }

        if (!window.confirm(confirmMessage)) {
            return;
        }

        setDeletingDepositId(depositId);
        setDeleteError(null);

        try {
            await deleteDeposit(depositId);
            // Reload data to reflect deletion
            loadData();
            // Optionally show a success message briefly
        } catch (err) {
            console.error("Failed to delete deposit:", err);
            setDeleteError(err instanceof Error ? err.message : 'Failed to delete the deposit.');
        } finally {
            setDeletingDepositId(null);
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

    // Toggle deposit item expansion
    const toggleDepositExpansion = (key: string) => {
        setExpandedDepositKeys(prev => {
            const newSet = new Set(prev);
            if (newSet.has(key)) {
                newSet.delete(key);
            } else {
                newSet.add(key);
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
        } catch (err) {
            console.error("Failed to delete spending group job:", err);
            setDeleteError(err instanceof Error ? err.message : 'Failed to delete the spending group.');
        } finally {
            setDeletingJobId(null); // Clear deleting state for the job
        }
    };

    // --- Render Logic ---

    // Helper to render a deposit item
    const renderDepositItem = (item: DepositItem) => {
        // Key uses original template ID + occurrence date for uniqueness
        const depositId = item.id; // This is the template ID
        const itemKey = `dep-${depositId}-${item.date}`; // Calculate key for the element
        const description = item.description ?? 'N/A';
        const amount = item.amount ?? 0;
        const isRecurring = item.is_recurring ?? false; // From the template
        const recurrencePeriod = item.recurrence_period;
        const isDeleting = deletingDepositId === depositId;
        const disableActions = isDeleting || deletingJobId !== null || editingItemId !== null;
        const isExpanded = expandedDepositKeys.has(itemKey);

        return (
            // Apply the calculated key here
            <div key={itemKey} className="border border-green-200 bg-green-50 rounded-lg shadow-sm overflow-hidden">
                {/* --- Mobile View (Stacked) --- */}
                <div className="md:hidden p-3">
                    {/* Top Row: Icon, Description, Amount, Expander */}
                    <div className="flex justify-between items-start gap-2 cursor-pointer" onClick={() => toggleDepositExpansion(itemKey)}>
                        <div className="flex items-center space-x-3 flex-1 min-w-0">
                            <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6 text-green-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5l-3 3m0 0l-3-3m3 3V8" />
                            </svg>
                            <p className="text-sm font-medium text-green-800 break-words flex-1 min-w-0">
                                Deposit: <span className="text-gray-700 font-normal">{description}</span>
                            </p>
                        </div>
                        <div className="flex items-center space-x-1 flex-shrink-0">
                            <p className="text-lg font-semibold text-green-700">
                                +{formatCurrency(amount)}
                            </p>
                            {/* Expander Icon */}
                            <span className="text-gray-400 hover:text-gray-600 p-1">
                                {isExpanded ?
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" /></svg> :
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
                                }
                            </span>
                        </div>
                    </div>

                    {/* Collapsible Details */}
                    {isExpanded && (
                        <div className="mt-3 space-y-2 border-t border-green-100 pt-3">
                            {/* Date & Recurring Info */}
                            <div>
                                <span className="text-xs font-medium text-gray-500 uppercase">Date</span>
                                <p className="text-sm text-gray-600">
                                    {formatDate(item.date, { hour: undefined, minute: undefined })}
                                    {isRecurring && <span className="ml-2 text-xs italic">({recurrencePeriod || 'Recurring'})</span>}
                                </p>
                            </div>
                            {/* Actions */}
                            <div className="flex justify-end items-center space-x-2 pt-1">
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleEditDepositClick(depositId); }}
                                    disabled={disableActions}
                                    className="text-indigo-600 hover:text-indigo-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-1"
                                    title="Edit this deposit template"
                                >
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                    </svg>
                                </button>
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleDeleteDepositClick(depositId, isRecurring); }}
                                    disabled={disableActions}
                                    className="text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1"
                                    title={isRecurring ? "Delete recurring deposit template" : "Delete this deposit"}
                                >
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                    </svg>
                                    {isDeleting && <span className="ml-1 text-xs">(Deleting...)</span>}
                                </button>
                            </div>
                        </div>
                    )}
                </div>

                {/* --- Desktop View (Flex) --- */}
                <div className="hidden md:block">
                    {/* Header Row */}
                    <div className="flex justify-between items-center flex-wrap gap-2 p-3 cursor-pointer hover:bg-green-100" onClick={() => toggleDepositExpansion(itemKey)}>
                        {/* Left side: Icon, Description */}
                        <div className="flex items-center space-x-3 flex-1 min-w-0 mr-2">
                            <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6 text-green-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                <path strokeLinecap="round" strokeLinejoin="round" d="M17 9V7a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2m2 4h10a2 2 0 002-2v-6a2 2 0 00-2-2H9a2 2 0 00-2 2v6a2 2 0 002 2zm7-5l-3 3m0 0l-3-3m3 3V8" />
                            </svg>
                            <div className="flex-1 min-w-0">
                                <p className="text-sm font-medium text-green-800 break-words">
                                    Deposit: <span className="text-gray-700 font-normal">{description}</span>
                                </p>
                            </div>
                        </div>
                        {/* Right side: Amount & Expander */}
                        <div className="flex items-center space-x-2 flex-shrink-0">
                            <p className="text-lg font-semibold text-green-700">
                                +{formatCurrency(amount)}
                            </p>
                            {/* Expander Icon */}
                            <span className="text-gray-400 hover:text-gray-600 p-1">
                                {isExpanded ?
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" /></svg> :
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
                                }
                            </span>
                        </div>
                    </div>
                    {/* Collapsible Details */}
                    {isExpanded && (
                        <div className="p-3 border-t border-green-100 bg-white">
                             {/* Date & Recurring Info */}
                             <div className="mb-2">
                                <span className="text-xs font-medium text-gray-500 uppercase">Date</span>
                                <p className="text-sm text-gray-600">
                                    {formatDate(item.date, { hour: undefined, minute: undefined })}
                                    {isRecurring && <span className="ml-2 text-xs italic">({recurrencePeriod || 'Recurring'})</span>}
                                </p>
                            </div>
                            {/* Actions */}
                            <div className="flex justify-end items-center space-x-2">
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleEditDepositClick(depositId); }}
                                    disabled={disableActions}
                                    className="text-indigo-600 hover:text-indigo-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:ring-offset-1"
                                    title="Edit this deposit template"
                                >
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                    </svg>
                                </button>
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleDeleteDepositClick(depositId, isRecurring); }}
                                    disabled={disableActions}
                                    className="text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1"
                                    title={isRecurring ? "Delete recurring deposit template" : "Delete this deposit"}
                                >
                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                    </svg>
                                    {isDeleting && <span className="ml-1 text-xs">(Deleting...)</span>}
                                </button>
                            </div>
                        </div>
                    )}
                </div>
            </div>
        );
    };

    // Helper to render either display row or edit form for a SpendingItem
    const renderSpendingItemRow = (item: SpendingItem, partnerNameFromGroup: string | null | undefined) => {
        const isEditing = editingItemId === item.id && editFormData;

        // Common classes for the container
        const containerClasses = `block md:table-row ${isEditing ? 'bg-yellow-50' : 'bg-white'} border-b border-gray-200 md:border-none`;

        if (isEditing) {
            // --- Render Edit Form (Responsive) ---
            return (
                <div key={`${item.id}-edit`} className={containerClasses}>
                    {/* Description Input */}
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
                    {/* Category Select */}
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
                    {/* Amount (Read-only) */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap text-sm text-gray-500 md:text-right">
                         <span className="text-xs font-medium text-gray-500 uppercase md:hidden">Amount: </span>
                         {formatCurrency(item.amount)}
                    </div>
                    {/* Sharing Status Select */}
                    <div className="px-4 py-3 md:table-cell md:whitespace-nowrap">
                         <label htmlFor={`edit-share-${item.id}`} className="text-xs font-medium text-gray-500 uppercase md:hidden">Sharing</label>
                         <select
                            id={`edit-share-${item.id}`}
                            value={editFormData.sharing_status}
                            onChange={(e) => handleEditFormChange('sharing_status', e.target.value)}
                            className="mt-1 md:mt-0 w-full px-2 py-1 border border-gray-300 rounded text-sm bg-white"
                        >
                            {/* Dynamically display partner name from the group if available */}
                            <option value="Alone">Alone</option>
                            <option value="Shared">Shared {partnerNameFromGroup ? `with ${partnerNameFromGroup}` : ''}</option>
                            <option value="Paid by Partner">Paid by {partnerNameFromGroup || 'Partner'}</option>
                        </select>
                    </div>
                    {/* Actions (Save/Cancel) */}
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
                </div>
            );
        } else {
            // --- Render Display Row/Card (Responsive) ---
            return (
                <div key={item.id} className={`${containerClasses} p-3 md:p-0 md:border-b md:border-gray-200`}>

                    {/* Mobile View Structure (md:hidden) */}
                    <div className="md:hidden space-y-2">
                        {/* Description */}
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
                                    item.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' :
                                    item.sharing_status.startsWith('Shared') ? 'bg-green-100 text-green-800' :
                                    item.sharing_status.startsWith('Paid by') ? 'bg-yellow-100 text-yellow-800' :
                                    'bg-gray-100 text-gray-800'
                                }`}>
                                    {item.sharing_status}
                                </span>
                            </div>
                        </div>

                        {/* Action (Edit Button) */}
                        <div className="pt-2 text-right">
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
                            'bg-gray-100 text-gray-800'
                        }`}>
                            {item.sharing_status}
                        </span>
                    </div>
                    {/* Action (Edit Button) - Only for spendings */}
                    <div className="hidden md:table-cell px-4 py-3 whitespace-nowrap text-right text-sm font-medium">
                        <button
                            onClick={() => handleEditClick(item)}
                            disabled={editingItemId !== null} // Disable if any item is being edited
                            className={`text-indigo-600 hover:text-indigo-900 ${editingItemId !== null ? 'opacity-50 cursor-not-allowed' : ''}`}
                        >
                            Edit
                        </button>
                    </div>
                </div>
            );
        }
    };


    return (
        <div className="bg-white shadow-md rounded-lg w-full max-w-4xl">
            <div className="p-4">
                <div className="flex flex-wrap justify-between items-center mb-4 gap-2">
                    <h1 className="text-2xl font-bold text-gray-700">History</h1>
                    <button
                    onClick={onBack}
                    className="text-sm text-indigo-600 hover:text-indigo-800"
                >
                    &larr; Back
                </button>
            </div>

            {/* Display Total Balance */}
            {!isLoading && !error && historyItems.length > 0 && (
                 <div className="mb-4 text-center">
                    <span className="text-lg font-semibold text-gray-600">Current Balance: </span>
                    <span className={`text-xl font-bold ${totalBalance >= 0 ? 'text-green-600' : 'text-red-600'}`}>
                        {formatCurrency(totalBalance)}
                    </span>
                </div>
            )}

            {isLoading && <div className="text-center p-4">Loading history...</div>}
            {/* Display general loading/fetch error */}
            {error && !isLoading && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error loading history: {error}</div>}
            {/* Display edit error (for spendings) */}
            {editError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error saving spending changes: {editError}</div>}
            {/* Display delete error (combined for jobs and deposits) */}
            {deleteError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error during deletion: {deleteError}</div>}


            {!isLoading && !error && historyItems.length === 0 && (
                <div className="text-center text-gray-500 p-4">No history found. Try logging some expenses or deposits!</div>
            )}

            {/* Render combined history items from the flat list */}
            {!isLoading && !error && historyItems.length > 0 && (
                <div className="space-y-4">
                    {/* Add type annotation for 'item' and apply key directly */}
                    {historyItems.map((item: HistoryListItem) => {
                        // Removed unused key variable declaration

                        if (item.type === 'deposit') {
                            // Render Deposit Item - Cast to DepositItem for type safety
                            // Apply key directly to the rendered element in renderDepositItem
                            return renderDepositItem(item as DepositItem);
                        } else if (item.type === 'spending_group') {
                            // Render Spending Group
                            // Use optional chaining and provide defaults for safety
                            const jobId = item.job_id ?? 0;
                            const prompt = item.prompt ?? 'N/A';
                            const totalAmount = item.total_amount ?? 0;
                            const buyerName = item.buyer_name ?? 'Unknown';
                            const isAmbiguous = item.is_ambiguity_flagged ?? false;
                            const ambiguityReason = item.ambiguity_flag_reason;
                            const spendings = item.spendings ?? [];
                            // Extract partner name from the first spending item if available (assuming consistent partner for the group)
                            const partnerNameFromGroup = spendings.length > 0 ? spendings[0].partner_name : null;
                            const itemKey = `sg-${jobId}`; // Calculate key for the element

                            return (
                                // Apply the calculated key here
                                <div key={itemKey} className="border border-gray-200 rounded-lg shadow-sm overflow-hidden">
                                    {/* Transaction Group Header - Make clickable */}
                                    <div
                                        className="bg-gray-50 p-3 border-b border-gray-200 cursor-pointer hover:bg-gray-100"
                                        onClick={() => toggleGroupExpansion(jobId)}
                                    >
                                        <div className="flex justify-between items-center flex-wrap gap-2">
                                            {/* Left side: Prompt, Date, Buyer, Total */}
                                            <div className="flex items-center space-x-3 flex-1 min-w-0 mr-2">
                                                {/* Spending Icon */}
                                                <svg xmlns="http://www.w3.org/2000/svg" className="h-6 w-6 text-red-600 flex-shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth="2">
                                                  <path strokeLinecap="round" strokeLinejoin="round" d="M3 10h18M7 15h1m4 0h1m-7 4h12a3 3 0 003-3V8a3 3 0 00-3-3H6a3 3 0 00-3 3v8a3 3 0 003 3z" />
                                                </svg>
                                                <div>
                                                    <p className="text-sm font-medium text-indigo-600 break-words" title={prompt}>
                                                        Spending: <span className="text-gray-700 font-normal">{prompt}</span>
                                                    </p>
                                                    <p className="text-xs text-gray-500">
                                                        {formatDate(item.date)} by <span className="font-medium">{buyerName}</span> - Total: <span className="font-semibold text-red-700">{formatCurrency(totalAmount)}</span>
                                                    </p>
                                                </div>
                                            </div>
                                            {/* Right side: Ambiguity flag, Delete Button, Expander Icon */}
                                            <div className="flex items-center flex-shrink-0 space-x-2">
                                                {isAmbiguous && (
                                                    <span
                                                        className="px-2 py-1 inline-flex text-xs leading-4 font-semibold rounded-full bg-yellow-100 text-yellow-800 cursor-help"
                                                        title={`Ambiguity Reason: ${ambiguityReason || 'No reason provided'}`}
                                                        onClick={(e) => e.stopPropagation()}
                                                    >
                                                        ⚠️ Ambiguous
                                                    </span>
                                                )}
                                                <button
                                                    onClick={(e) => { e.stopPropagation(); handleDeleteJob(jobId); }}
                                                    disabled={deletingJobId === jobId || editingItemId !== null}
                                                    className={`text-red-600 hover:text-red-800 disabled:opacity-50 disabled:cursor-not-allowed p-1 rounded focus:outline-none focus:ring-2 focus:ring-red-500 focus:ring-offset-1`}
                                                    title="Delete this entire spending group"
                                                >
                                                    <svg xmlns="http://www.w3.org/2000/svg" className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                                                        <path strokeLinecap="round" strokeLinejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                                    </svg>
                                                    {deletingJobId === jobId && <span className="ml-1 text-xs">(Deleting...)</span>}
                                                </button>
                                                {/* Expander Icon */}
                                                <span className="text-gray-400 hover:text-gray-600 cursor-pointer p-1" onClick={(e) => { e.stopPropagation(); toggleGroupExpansion(jobId); }}>
                                                    {expandedGroupIds.has(jobId) ?
                                                        <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M5 15l7-7 7 7" /></svg> :
                                                        <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}><path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" /></svg>
                                                    }
                                                </span>
                                            </div>
                                        </div>
                                    </div>

                                    {/* Spending Items Container (Conditional Rendering) */}
                                    {expandedGroupIds.has(jobId) && (
                                        <div className="bg-white">
                                            {/* Pass partner name to spending item renderer */}
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
                                                    {spendings.map((sp: SpendingItem) => renderSpendingItemRow(sp, partnerNameFromGroup))}
                                                    {spendings.length === 0 && (
                                                        <tr className="md:table-row">
                                                            <td colSpan={5} className="md:table-cell px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</td>
                                                        </tr>
                                                    )}
                                                </tbody>
                                            </table>
                                            <div className="md:hidden space-y-3 p-2 bg-gray-50">
                                                 {spendings.map((sp: SpendingItem) => renderSpendingItemRow(sp, partnerNameFromGroup))}
                                                 {spendings.length === 0 && (
                                                    <div className="px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</div>
                                                )}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            );
                        } else {
                            // Handle unknown item types if necessary
                            console.warn("Unknown history item type:", item.type);
                            const itemKey = `unknown-${item.date}`; // Calculate key for the element
                            return <div key={itemKey} className="text-red-500">Unknown item type encountered</div>;
                        }
                    })}
                </div>
            )}
            </div>
        </div>
    );
}

export default HistoryList;
