import { useState, useEffect, useCallback } from 'react';
import { fetchTransferStatus, recordTransfer } from './api';
import { TransferStatusResponse } from './types';

interface TransferPageProps {
    onBack: () => void; // Function to navigate back
}

function TransferPage({ onBack }: TransferPageProps) {
    const [status, setStatus] = useState<TransferStatusResponse | null>(null);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);
    const [isRecording, setIsRecording] = useState<boolean>(false); // Loading state for record button
    const [recordError, setRecordError] = useState<string | null>(null);
    const [recordSuccess, setRecordSuccess] = useState<string | null>(null);

    const loadStatus = useCallback(() => {
        setIsLoading(true);
        setError(null);
        setRecordError(null); // Clear record errors on reload
        setRecordSuccess(null); // Clear success message on reload
        fetchTransferStatus()
            .then(data => {
                setStatus(data);
            })
            .catch(err => {
                console.error("Failed to load transfer status:", err);
                setError(err instanceof Error ? err.message : 'Failed to load transfer status.');
                setStatus(null);
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, []);

    useEffect(() => {
        loadStatus();
    }, [loadStatus]);

    const handleRecordTransfer = async () => {
        setIsRecording(true);
        setRecordError(null);
        setRecordSuccess(null);
        try {
            await recordTransfer();
            setRecordSuccess('Transfer recorded successfully! Balance is now settled.');
            // Reload status after recording
            loadStatus();
        } catch (err) {
            console.error("Failed to record transfer:", err);
            setRecordError(err instanceof Error ? err.message : 'Failed to record transfer.');
        } finally {
            setIsRecording(false);
        }
    };

    // Helper to format currency
    const formatCurrency = (amount: number) => {
        return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' }); // Adjust currency code if needed
    };

    const renderStatus = () => {
        if (isLoading) {
            return <div className="text-center p-4 text-gray-500">Loading transfer status...</div>;
        }
        if (error) {
            return <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error: {error}</div>;
        }
        if (!status) {
            return <div className="text-center p-4 text-gray-500">Could not load status.</div>;
        }

        let statusText = '';
        let statusColor = 'text-gray-700'; // Default color

        if (status.owed_by && status.owed_to) {
            // Someone owes someone
            statusText = `${status.owed_by} owes ${status.owed_to} ${formatCurrency(status.amount_owed)}`;
            // Check if the current user (assuming 'You' if name matches, needs user info from App context ideally) owes money
            // For now, we just display the names from the backend.
            // You might want to pass the current user's name from App.tsx to display "You owe..." or "...owes you"
            statusColor = 'text-orange-600'; // Indicate money needs to change hands
        } else {
            // Settled up
            statusText = `Settled up with ${status.partner_name}. No transfer needed.`;
            statusColor = 'text-green-600'; // Indicate settled state
        }

        return (
            <div className="text-center p-6 border border-gray-200 rounded-lg bg-gray-50">
                <p className={`text-xl font-semibold ${statusColor}`}>{statusText}</p>
            </div>
        );
    };

    const canRecordTransfer = status && (status.owed_by !== null || status.owed_to !== null);

    return (
        // Remove p-6, add p-4 inside
        <div className="bg-white shadow-md rounded-lg w-full max-w-md">
             <div className="p-4"> {/* Add inner padding */}
                <div className="flex flex-wrap justify-between items-center mb-6 gap-2"> {/* Allow wrapping */}
                    <h1 className="text-2xl font-bold text-gray-700">Transfer Status</h1>
                    <button
                    onClick={onBack}
                    className="text-sm text-indigo-600 hover:text-indigo-800"
                >
                    &larr; Back
                </button>
            </div>

            {renderStatus()}

            {recordError && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mt-4" role="alert">Error recording transfer: {recordError}</div>}
            {recordSuccess && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mt-4" role="alert">{recordSuccess}</div>}

            <div className="mt-6 text-center">
                <button
                    onClick={handleRecordTransfer}
                    disabled={isLoading || isRecording || !canRecordTransfer}
                    className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${(!canRecordTransfer || isLoading || isRecording) ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
                >
                    {isRecording ? 'Recording...' : 'Record Transfer (Settle Balance)'}
                </button>
                {!canRecordTransfer && !isLoading && !error && (
                     <p className="text-sm text-gray-500 mt-2">Balance is already settled.</p>
                )}
            </div>
                <p className="text-xs text-gray-500 mt-4 text-center italic">
                    This action marks all currently shared expenses between you and {status?.partner_name || 'your partner'} as settled. It assumes the necessary bank transfer has been made outside of this app.
                </p>
            </div> {/* Close inner padding div */}
        </div>
    );
}

export default TransferPage;
