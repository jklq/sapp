import { useState, useEffect, useMemo } from 'react';
import { Pie } from 'react-chartjs-2';
import {
    Chart as ChartJS,
    ArcElement,
    Tooltip,
    Legend,
    Title, // <-- Add this import
    ChartData,
    ChartOptions,
} from 'chart.js';
import { fetchLastMonthSpendingStats } from './api';
import { CategorySpendingStat } from './types';

// Register Chart.js components
ChartJS.register(ArcElement, Tooltip, Legend, Title); // <-- Add Title here

interface StatsPageProps {
    onBack: () => void; // Function to navigate back
}

// Helper to generate distinct colors (simple version)
const generateColors = (numColors: number): string[] => {
    const colors = [];
    const hueStep = 360 / numColors;
    for (let i = 0; i < numColors; i++) {
        // Use HSL color space for better distribution
        // Keep saturation and lightness constant, vary hue
        colors.push(`hsl(${i * hueStep}, 70%, 60%)`);
    }
    return colors;
};

// Helper to format currency
const formatCurrency = (amount: number) => {
    return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' }); // Adjust currency code if needed
};


function StatsPage({ onBack }: StatsPageProps) {
    const [statsData, setStatsData] = useState<CategorySpendingStat[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        setIsLoading(true);
        setError(null);
        fetchLastMonthSpendingStats()
            .then((data: CategorySpendingStat[]) => { // Add type for data
                setStatsData(data);
            })
            .catch((err: unknown) => { // Add type for err
                console.error("Failed to load spending stats:", err);
                // Use type guard before accessing message
                setError(err instanceof Error ? err.message : 'Failed to load stats.');
                setStatsData([]); // Clear data on error
            })
            .finally(() => {
                setIsLoading(false);
            });
    }, []); // Fetch data on component mount

    // Prepare data for the Pie chart
    const chartData: ChartData<'pie'> = useMemo(() => {
        const labels = statsData.map(item => item.category_name);
        const data = statsData.map(item => item.total_amount);
        const backgroundColors = generateColors(statsData.length);

        return {
            labels: labels,
            datasets: [
                {
                    label: 'Spending',
                    data: data,
                    backgroundColor: backgroundColors,
                    borderColor: backgroundColors.map(color => color.replace('60%)', '50%)')), // Slightly darker border
                    borderWidth: 1,
                },
            ],
        };
    }, [statsData]);

    // Chart options
    const chartOptions: ChartOptions<'pie'> = {
        responsive: true,
        maintainAspectRatio: false, // Allow chart to fill container height
        plugins: {
            legend: {
                position: 'top' as const, // Position legend at the top
            },
            tooltip: {
                callbacks: {
                    label: function (context) {
                        let label = context.label || '';
                        if (label) {
                            label += ': ';
                        }
                        const value = context.parsed || 0;
                        label += formatCurrency(value); // Format tooltip value as currency
                        return label;
                    }
                }
            },
            title: {
                display: true,
                text: 'Spending Distribution by Category (Last 30 Days)', // Made title more descriptive
                font: {
                    size: 16, // Increased font size slightly for visibility
                },
                padding: {
                    top: 10,
                    bottom: 20 // Added more padding below title
                }
            },
        },
    };

    return (
        <div className="bg-white shadow-md rounded-lg w-full max-w-2xl">
            <div className="p-4">
                <div className="flex flex-wrap justify-between items-center mb-4 gap-2">
                    <h1 className="text-2xl font-bold text-gray-700">Spending Stats</h1>
                    <button
                        onClick={onBack}
                        className="text-sm text-indigo-600 hover:text-indigo-800"
                    >
                        &larr; Back
                    </button>
                </div>

                {isLoading && <div className="text-center p-4 text-gray-500">Loading stats...</div>}
                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error: {error}</div>}

                {!isLoading && !error && statsData.length === 0 && (
                    <div className="text-center p-4 text-gray-500">No spending data found for the last 30 days.</div>
                )}

                {!isLoading && !error && statsData.length > 0 && (
                    // Container to control chart size
                    <div className="relative h-64 md:h-96">
                        <Pie data={chartData} options={chartOptions} />
                    </div>
                )}
            </div>
        </div>
    );
}

export default StatsPage;
