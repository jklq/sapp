import { useState, useEffect, Fragment } from 'react'; // Added Fragment
import { fetchSpendings } from './api';
import { TransactionGroup, SpendingItem, GroupedSpendingsResponse } from './types'; // Import new types

interface SpendingsListProps {
    onBack: () => void;
}

function SpendingsList({ onBack }: SpendingsListProps) {
    // State now holds an array of TransactionGroup
    const [transactionGroups, setTransactionGroups] = useState<GroupedSpendingsResponse>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setIsLoading(true);
        setError(null);
        fetchSpendings() // Calls the updated API function
            .then(data => {
                setTransactionGroups(data);
            })
            .catch(err => {
                console.error("Failed to fetch spendings:", err);
                setError(err instanceof Error ? err.message : 'Failed to load spending data.');
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, []); // Fetch on mount

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


            {isLoading && <div className="text-center p-4">Loading spendings...</div>}
            {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}

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
                                            {/* Buyer might be redundant if always the same as job submitter */}
                                            {/* <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Paid By</th> */}
                                            <th scope="col" className="px-4 py-2 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Sharing</th>
                                        </tr>
                                    </thead>
                                    <tbody className="bg-white divide-y divide-gray-200">
                                        {group.spendings.map((item) => (
                                            <tr key={item.id}>
                                                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900">{item.description || '-'}</td>
                                                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">{item.category_name}</td>
                                                <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-900 text-right">{formatCurrency(item.amount)}</td>
                                                {/* <td className="px-4 py-3 whitespace-nowrap text-sm text-gray-500">{item.buyer_name}</td> */}
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
                                            </tr>
                                        ))}
                                        {group.spendings.length === 0 && (
                                            <tr>
                                                <td colSpan={4} className="px-4 py-3 text-center text-sm text-gray-500 italic">No spending items generated for this job.</td>
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
