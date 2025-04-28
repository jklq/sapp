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
import { fetchSpendingStats } from './api'; // Use the updated function name
import { CategorySpendingStat } from './types';

// Register Chart.js components
ChartJS.register(ArcElement, Tooltip, Legend, Title); // <-- Add Title here

interface StatsPageProps {
    onBack: () => void; // Function to navigate back
}

// Helper function to generate a consistent color from a string (category name)
const stringToColor = (str: string): string => {
    let hash = 0;
    for (let i = 0; i < str.length; i++) {
        hash = str.charCodeAt(i) + ((hash << 5) - hash);
        hash = hash & hash; // Convert to 32bit integer
    }

    // Use golden angle approximation (137.5 degrees) for better hue distribution
    const hue = (hash * 137.508) % 360; // Use golden angle

    // Introduce slight variations in saturation and lightness based on hash
    // Ensure values stay within reasonable bounds (e.g., Saturation 60-80%, Lightness 50-70%)
    const saturation = 60 + (hash % 21); // Vary saturation between 60% and 80%
    const lightness = 50 + (hash % 21); // Vary lightness between 50% and 70%

    return `hsl(${hue.toFixed(0)}, ${saturation}%, ${lightness}%)`;
};


// Helper to format currency
const formatCurrency = (amount: number) => {
    return amount.toLocaleString(undefined, { style: 'currency', currency: 'NOK' }); // Adjust currency code if needed
};


function StatsPage({ onBack }: StatsPageProps) {
    const [statsData, setStatsData] = useState<CategorySpendingStat[]>([]);
    const [isLoading, setIsLoading] = useState<boolean>(false); // Start not loading initially
    const [error, setError] = useState<string | null>(null);

    // State for date range selection
    const [startDate, setStartDate] = useState<string>(() => {
        const date = new Date();
        date.setDate(date.getDate() - 30); // Default start date: 30 days ago
        return date.toISOString().split('T')[0];
    });
    const [endDate, setEndDate] = useState<string>(() => {
        const date = new Date(); // Default end date: today
        return date.toISOString().split('T')[0];
    });

    // Function to fetch data based on current date state
    const loadStats = () => {
        setIsLoading(true);
        setError(null);
        setStatsData([]); // Clear previous data

        fetchSpendingStats(startDate, endDate) // Call updated API function
            .then((data: CategorySpendingStat[]) => {
                setStatsData(data);
            })
            .catch((err: unknown) => { // Add type for err
                console.error("Failed to load spending stats:", err);
                // Use type guard before accessing message
                setError(err instanceof Error ? err.message : 'Failed to load stats.');
                setStatsData([]); // Clear data on error
            })
            .finally(() => {
                // The error state is already set correctly in the .catch block.
                // We only need to set loading to false here.
                setIsLoading(false);
            });
    };

    // Fetch data on initial mount and when dates change
    useEffect(() => {
        loadStats();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [startDate, endDate]); // Re-fetch when dates change

    // Prepare data for the Pie chart
    const chartData: ChartData<'pie'> = useMemo(() => {
        const labels = statsData.map(item => item.category_name);
        const data = statsData.map(item => item.total_amount);
        // Generate colors based on category names for consistency
        const backgroundColors = labels.map(label => stringToColor(label));
        // Generate slightly darker border colors based on the background colors
        const borderColors = backgroundColors.map(color => {
            // Example: Decrease lightness by 10% for the border
            // This requires parsing HSL, modifying, and formatting back.
            // Simple approach: use the same color or a fixed border color.
            // Let's use a slightly darker version by adjusting lightness.
            try {
                const match = color.match(/hsl\((\d+),\s*(\d+)%,\s*(\d+)%\)/);
                if (match) {
                    const [, hue, saturation, lightness] = match;
                    const darkerLightness = Math.max(0, parseInt(lightness, 10) - 10); // Decrease lightness
                    return `hsl(${hue}, ${saturation}%, ${darkerLightness}%)`;
                }
            } catch (e) { /* ignore parsing error, fallback */ }
            return color; // Fallback to same color if parsing fails
        });


        return {
            labels: labels,
            datasets: [
                {
                    label: 'Spending',
                    data: data,
                    backgroundColor: backgroundColors, // Use category-based colors
                    borderColor: borderColors,         // Use derived border colors
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
                // Make title dynamic or more generic
                text: `Spending Distribution (${startDate} to ${endDate})`,
                font: {
                    size: 16,
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

                {/* Date Range Selection */}
                <div className="mb-4 flex flex-col sm:flex-row justify-center items-center gap-4">
                    <div>
                        <label htmlFor="startDate" className="block text-sm font-medium text-gray-700 mr-2">Start Date:</label>
                        <input
                            type="date"
                            id="startDate"
                            value={startDate}
                            onChange={(e) => setStartDate(e.target.value)}
                            className="mt-1 sm:mt-0 px-3 py-1 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>
                    <div>
                        <label htmlFor="endDate" className="block text-sm font-medium text-gray-700 mr-2">End Date:</label>
                        <input
                            type="date"
                            id="endDate"
                            value={endDate}
                            onChange={(e) => setEndDate(e.target.value)}
                            className="mt-1 sm:mt-0 px-3 py-1 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
                        />
                    </div>
                    {/* Optional: Add a button to trigger fetch manually instead of useEffect */}
                    {/* <button onClick={loadStats} className="px-4 py-1 bg-indigo-600 text-white rounded-md text-sm hover:bg-indigo-700">Load Stats</button> */}
                </div>


                {isLoading && <div className="text-center p-4 text-gray-500">Loading stats...</div>}
                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error: {error}</div>}

                {!isLoading && !error && statsData.length === 0 && (
                    <div className="text-center p-4 text-gray-500">No spending data found for the selected period.</div>
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
