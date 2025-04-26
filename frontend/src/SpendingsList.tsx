import { useState, useEffect } from 'react';
import { fetchSpendings } from './api';
import { SpendingDetail } from './types';

interface SpendingsListProps {
    // Props if needed, e.g., function to switch back to the form
    onBack: () => void;
}

function SpendingsList({ onBack }: SpendingsListProps) {
    const [spendings, setSpendings] = useState<SpendingDetail[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setIsLoading(true);
        setError(null);
        fetchSpendings()
            .then(data => {
                setSpendings(data);
            })
            .catch(err => {
                console.error("Failed to fetch spendings:", err);
                setError(err instanceof Error ? err.message : 'Failed to load spending data.');
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, []); // Fetch on mount

    // Helper to format date string
    const formatDate = (dateString: string) => {
        try {
            return new Date(dateString).toLocaleDateString(undefined, {
                year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
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

            {!isLoading && !error && spendings.length === 0 && (
                <div className="text-center text-gray-500 p-4">No spendings recorded yet.</div>
            )}

            {!isLoading && !error && spendings.length > 0 && (
                <div className="overflow-x-auto"> {/* Make table scrollable on small screens */}
                    <table className="min-w-full divide-y divide-gray-200">
                        <thead className="bg-gray-50">
                            <tr>
                                <th scope="col" className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Date</th>
                                <th scope="col" className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Description</th>
                                <th scope="col" className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Category</th>
                                <th scope="col" className="px-4 py-3 text-right text-xs font-medium text-gray-500 uppercase tracking-wider">Amount</th>
                                <th scope="col" className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Paid By</th>
                                <th scope="col" className="px-4 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Sharing Status</th>
                            </tr>
                        </thead>
                        <tbody className="bg-white divide-y divide-gray-200">
                            {spendings.map((spending) => (
                                <tr key={spending.id}>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-500">{formatDate(spending.created_at)}</td>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-900">{spending.description || '-'}</td>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-500">{spending.category_name}</td>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-900 text-right">{formatCurrency(spending.amount)}</td>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-500">{spending.buyer_name}</td>
                                    <td className="px-4 py-4 whitespace-nowrap text-sm text-gray-500">
                                        <span className={`px-2 inline-flex text-xs leading-5 font-semibold rounded-full ${
                                            spending.sharing_status === 'Alone' ? 'bg-blue-100 text-blue-800' :
                                            spending.sharing_status.startsWith('Shared') ? 'bg-green-100 text-green-800' :
                                            spending.sharing_status.startsWith('Paid by') ? 'bg-yellow-100 text-yellow-800' :
                                            'bg-gray-100 text-gray-800' // Fallback
                                        }`}>
                                            {spending.sharing_status}
                                        </span>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </div>
    );
}

export default SpendingsList;
