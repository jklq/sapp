import { useState, useEffect, useMemo, useCallback } from 'react'; // Added useCallback
import { Pie, Line } from 'react-chartjs-2'; // Added Line
import {
    Chart as ChartJS,
    ArcElement, // For Pie chart
    LineElement, // For Line chart
    CategoryScale, // For X axis (dates)
    LinearScale, // For Y axis (amount)
    PointElement, // For points on the line
    Tooltip,
    Legend, // Common
    Title, // Common
    ChartData,
    ChartOptions,
} from 'chart.js';
// Import necessary API functions and types
import { fetchSpendingStats, fetchDepositStats, fetchHistory } from './api';
import { CategorySpendingStat, DepositStatsResponse, HistoryResponse, HistoryListItem, SpendingItem } from './types';

// Register Chart.js components
ChartJS.register(
    ArcElement, // Pie
    LineElement, // Line
    CategoryScale, // Line X axis
    LinearScale, // Line Y axis
    PointElement, // Line points
    Tooltip, // Common
    Legend, // Common
    Title // Common
);

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
    // State for Pie Chart (Spending Categories)
    const [categoryStatsData, setCategoryStatsData] = useState<CategorySpendingStat[]>([]);
    // State for Line Chart (Income vs Cumulative Spending)
    const [depositTotal, setDepositTotal] = useState<number | null>(null);
    const [historyItems, setHistoryItems] = useState<HistoryListItem[]>([]); // For cumulative spending calculation

    // Combined Loading/Error States
    const [isLoading, setIsLoading] = useState<boolean>(true); // Start loading initially
    const [error, setError] = useState<string | null>(null);

    // State for date range selection (common for both charts)
    const [startDate, setStartDate] = useState<string>(() => {
        const date = new Date();
        date.setDate(date.getDate() - 30); // Default start date: 30 days ago
        return date.toISOString().split('T')[0];
    });
    const [endDate, setEndDate] = useState<string>(() => {
        const date = new Date(); // Default end date: today
        return date.toISOString().split('T')[0];
    });

    // Function to fetch all data needed for the page
    const loadAllStats = useCallback(() => {
        setIsLoading(true);
        setError(null);
        setCategoryStatsData([]); // Clear previous data
        setDepositTotal(null);
        setHistoryItems([]);

        // Fetch all required data in parallel
        Promise.all([
            fetchSpendingStats(startDate, endDate),
            fetchDepositStats(startDate, endDate),
            fetchHistory() // Fetch full history for cumulative calculation
        ])
        .then(([categoryData, depositData, historyData]) => {
            setCategoryStatsData(categoryData);
            setDepositTotal(depositData.total_amount);
            setHistoryItems(historyData.history || []); // Store the flat history list
        })
        .catch((err: unknown) => {
            console.error("Failed to load stats data:", err);
            setError(err instanceof Error ? err.message : 'Failed to load required stats data.');
            // Clear all data on error
            setCategoryStatsData([]);
            setDepositTotal(null);
            setHistoryItems([]);
        })
        .finally(() => {
            setIsLoading(false);
        });
    // Include dependencies for useCallback
    }, [startDate, endDate]);

    // Fetch data on initial mount and when dates change
    useEffect(() => {
        loadAllStats();
    }, [loadAllStats]); // Re-fetch when loadAllStats changes (due to date changes)

    // --- Pie Chart Data Preparation ---
    const pieChartData: ChartData<'pie'> = useMemo(() => {
        const labels = categoryStatsData.map(item => item.category_name);
        const data = categoryStatsData.map(item => item.total_amount);
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
    }, [categoryStatsData]);

    // --- Pie Chart Options ---
    const pieChartOptions: ChartOptions<'pie'> = {
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

    // --- Line Chart Data Preparation (Income vs Cumulative Spending) ---
    const lineChartData = useMemo(() => {
        if (!historyItems || depositTotal === null) {
            return { labels: [], datasets: [] }; // Return empty data if not loaded
        }

        const dailyUserSpending: { [date: string]: number } = {};
        const start = new Date(startDate + 'T00:00:00Z'); // Ensure UTC parsing
        const end = new Date(endDate + 'T00:00:00Z'); // Ensure UTC parsing
        const today = new Date();
        today.setUTCHours(0, 0, 0, 0); // Normalize today to UTC start of day

        // 1. Calculate user's spending share per day within the range
        historyItems.forEach(item => {
            if (item.type === 'spending_group') {
                const groupDate = new Date(item.date); // Assuming item.date is ISO string
                groupDate.setUTCHours(0, 0, 0, 0); // Normalize group date

                // Check if group date is within the selected range [start, end]
                if (groupDate >= start && groupDate <= end) {
                    const dateString = groupDate.toISOString().split('T')[0]; // YYYY-MM-DD format

                    let groupCostForUser = 0;
                    if (item.spendings && Array.isArray(item.spendings)) {
                        item.spendings.forEach((spending: SpendingItem) => {
                            const spendingAmount = Number(spending.amount) || 0;
                            // Calculate user's share based on status
                            if (spending.sharing_status === 'Alone') {
                                groupCostForUser += spendingAmount;
                            } else if (spending.sharing_status?.startsWith('Shared')) {
                                groupCostForUser += spendingAmount / 2.0;
                            } else if (spending.sharing_status?.startsWith('Paid by')) {
                                groupCostForUser += 0;
                            } else {
                                // Default/fallback: assume user pays full amount
                                groupCostForUser += spendingAmount;
                            }
                        });
                    }
                    dailyUserSpending[dateString] = (dailyUserSpending[dateString] || 0) + groupCostForUser;
                }
            }
        });

        // 2. Generate labels (all dates in the range) and calculate cumulative spending
        const labels: string[] = [];
        const cumulativeSpendingData: (number | null)[] = []; // Use null for future dates
        let cumulativeAmount = 0;
        const currentDate = new Date(start); // Start iteration from start date

        while (currentDate <= end) {
            const dateString = currentDate.toISOString().split('T')[0];
            labels.push(dateString);

            // Check if the current iteration date is in the future relative to 'today'
            const isFutureDate = currentDate > today;

            if (isFutureDate) {
                // If the date is in the future, set cumulative spending to null
                cumulativeSpendingData.push(null);
            } else {
                // Otherwise, add today's spending (if any) and push the cumulative amount
                cumulativeAmount += (dailyUserSpending[dateString] || 0);
                // Round to 2 decimal places
                cumulativeSpendingData.push(Math.round(cumulativeAmount * 100) / 100);
            }

            // Move to the next day
            currentDate.setUTCDate(currentDate.getUTCDate() + 1);
        }

        return {
            labels: labels,
            datasets: [
                {
                    label: 'Total Income (Period)',
                    data: Array(labels.length).fill(depositTotal), // Constant line
                    borderColor: 'rgb(75, 192, 192)', // Greenish color
                    backgroundColor: 'rgba(75, 192, 192, 0.1)',
                    tension: 0.1,
                    pointRadius: 0, // No points for the income line
                    borderWidth: 2,
                    fill: false, // Don't fill area under income line
                },
                {
                    label: 'Cumulative Spending (Your Share)',
                    data: cumulativeSpendingData,
                    borderColor: 'rgb(255, 99, 132)', // Red color
                    backgroundColor: 'rgba(255, 99, 132, 0.1)',
                    tension: 0.1,
                    pointRadius: 2, // Small points for spending line
                    borderWidth: 2,
                    fill: true, // Fill area under spending line
                    spanGaps: false, // Do not connect lines across null points (future dates)
                },
            ],
        };
    }, [historyItems, depositTotal, startDate, endDate]);

    // --- Line Chart Options ---
    const lineChartOptions: ChartOptions<'line'> = {
        responsive: true,
        maintainAspectRatio: false,
        scales: {
            y: {
                beginAtZero: true,
                title: {
                    display: true,
                    text: 'Amount (NOK)', // Adjust currency if needed
                },
                ticks: {
                    // Format Y-axis ticks as currency
                    callback: function(value) {
                        // Ensure value is a number before formatting
                        if (typeof value === 'number') {
                            return formatCurrency(value);
                        }
                        return value;
                    }
                }
            },
            x: {
                title: {
                    display: true,
                    text: 'Date',
                },
            }
        },
        plugins: {
            legend: {
                position: 'top' as const,
            },
            title: {
                display: true,
                text: `Income vs Cumulative Spending (${startDate} to ${endDate})`,
                font: { size: 16 },
                padding: { top: 10, bottom: 20 }
            },
            tooltip: {
                mode: 'index', // Show tooltips for all datasets at the same index
                intersect: false, // Show tooltip even when not hovering directly over point
                callbacks: {
                    label: function (context) {
                        let label = context.dataset.label || '';
                        if (label) {
                            label += ': ';
                        }
                        const value = context.parsed.y;
                        if (value !== null) {
                             // Format tooltip value as currency
                            label += formatCurrency(value);
                        } else {
                            label += 'N/A'; // Indicate future data
                        }
                        return label;
                    }
                }
            },
        },
        interaction: {
            mode: 'index', // Enable interactions based on the x-axis index
            intersect: false,
        },
    };


    return (
        // Increase max-width to accommodate two charts better
        <div className="bg-white shadow-md rounded-lg w-full max-w-4xl">
            <div className="p-4">
                <div className="flex flex-wrap justify-between items-center mb-6 gap-2"> {/* Increased bottom margin */}
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

                {/* Loading and Error States */}
                {isLoading && <div className="text-center p-4 text-gray-500">Loading stats...</div>}
                {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">Error: {error}</div>}

                {/* Charts Section - Render only if not loading and no error */}
                {!isLoading && !error && (
                    <div className="space-y-8"> {/* Add space between charts */}

                        {/* Pie Chart Section */}
                        <div>
                            <h2 className="text-lg font-semibold text-gray-600 mb-2 text-center">Spending Breakdown by Category</h2>
                            {categoryStatsData.length === 0 ? (
                                <div className="text-center p-4 text-gray-500">No spending data found for the pie chart.</div>
                            ) : (
                                <div className="relative h-64 md:h-80"> {/* Adjusted height */}
                                    <Pie data={pieChartData} options={pieChartOptions} />
                                </div>
                            )}
                        </div>

                        {/* Line Chart Section */}
                        <div>
                             <h2 className="text-lg font-semibold text-gray-600 mb-2 text-center">Income vs. Cumulative Spending</h2>
                            {(depositTotal === null || historyItems.length === 0) && !isLoading ? (
                                 <div className="text-center p-4 text-gray-500">Insufficient data for income vs. spending chart.</div>
                            ) : (
                                <div className="relative h-72 md:h-96"> {/* Adjusted height */}
                                    <Line data={lineChartData} options={lineChartOptions} />
                                </div>
                            )}
                        </div>

                    </div>
                )}
            </div>
        </div>
    );
}

export default StatsPage;
