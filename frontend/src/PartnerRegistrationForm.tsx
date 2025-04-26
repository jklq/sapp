import { useState, FormEvent } from 'react';
import { registerPartners } from './api';
import { PartnerRegistrationPayload, UserRegistrationDetails } from './types';

interface PartnerRegistrationFormProps {
    onRegistrationSuccess: () => void; // Callback to navigate back to login or show success
    onBackToLogin: () => void; // Callback to go back to login view
}

function PartnerRegistrationForm({ onRegistrationSuccess, onBackToLogin }: PartnerRegistrationFormProps) {
    const initialUserState: UserRegistrationDetails = { username: '', password: '', first_name: '' };
    const [user1, setUser1] = useState<UserRegistrationDetails>(initialUserState);
    const [user2, setUser2] = useState<UserRegistrationDetails>(initialUserState);
    const [isLoading, setIsLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [successMessage, setSuccessMessage] = useState<string | null>(null);

    const handleInputChange = (userIndex: 1 | 2, field: keyof UserRegistrationDetails, value: string) => {
        const setUser = userIndex === 1 ? setUser1 : setUser2;
        setUser(prev => ({ ...prev, [field]: value }));
    };

    const handleSubmit = async (event: FormEvent) => {
        event.preventDefault();
        setError(null);
        setSuccessMessage(null);
        setIsLoading(true);

        // Basic Frontend Validation (more robust validation happens on backend)
        if (!user1.username || !user1.password || !user1.first_name || !user2.username || !user2.password || !user2.first_name) {
            setError('All fields are required for both users.');
            setIsLoading(false);
            return;
        }
        if (user1.username === user2.username) {
            setError('Usernames must be different.');
            setIsLoading(false);
            return;
        }
        if (user1.password.length < 6 || user2.password.length < 6) {
            setError('Passwords must be at least 6 characters long.');
            setIsLoading(false);
            return;
        }

        const payload: PartnerRegistrationPayload = { user1, user2 };

        try {
            const response = await registerPartners(payload);
            setSuccessMessage(`${response.message}. You can now log in.`);
            // Optionally clear form or call success callback after a delay
            setTimeout(() => {
                onRegistrationSuccess(); // e.g., navigate back to login
            }, 2000); // Wait 2 seconds before navigating
        } catch (err) {
            console.error("Registration failed:", err);
            setError(err instanceof Error ? err.message : 'An unknown registration error occurred.');
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="min-h-screen bg-gray-100 flex items-center justify-center">
            <div className="bg-white shadow-md rounded-lg w-full max-w-2xl"> {/* Wider card */}
                <div className="p-6"> {/* Increased padding */}
                    <div className="flex justify-between items-center mb-6">
                        <h1 className="text-2xl font-bold text-center text-gray-700">Register Partners</h1>
                        <button
                            onClick={onBackToLogin}
                            className="text-sm text-indigo-600 hover:text-indigo-800"
                        >
                            &larr; Back to Login
                        </button>
                    </div>

                    {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
                    {successMessage && <div className="bg-green-100 border border-green-400 text-green-700 px-4 py-3 rounded relative mb-4" role="alert">{successMessage}</div>}

                    <form onSubmit={handleSubmit} className="space-y-6">
                        <div className="grid grid-cols-1 md:grid-cols-2 gap-6"> {/* Grid layout */}
                            {/* User 1 Section */}
                            <div className="space-y-4 border border-gray-200 p-4 rounded">
                                <h2 className="text-lg font-semibold text-gray-600">Partner 1</h2>
                                <div>
                                    <label htmlFor="user1-username" className="block text-sm font-medium text-gray-700">Username</label>
                                    <input
                                        type="text" id="user1-username" value={user1.username}
                                        onChange={(e) => handleInputChange(1, 'username', e.target.value)} required
                                        className="mt-1 block w-full input-style" autoComplete="new-username"
                                    />
                                </div>
                                <div>
                                    <label htmlFor="user1-firstname" className="block text-sm font-medium text-gray-700">First Name</label>
                                    <input
                                        type="text" id="user1-firstname" value={user1.first_name}
                                        onChange={(e) => handleInputChange(1, 'first_name', e.target.value)} required
                                        className="mt-1 block w-full input-style"
                                    />
                                </div>
                                <div>
                                    <label htmlFor="user1-password"className="block text-sm font-medium text-gray-700">Password</label>
                                    <input
                                        type="password" id="user1-password" value={user1.password}
                                        onChange={(e) => handleInputChange(1, 'password', e.target.value)} required minLength={6}
                                        className="mt-1 block w-full input-style" autoComplete="new-password"
                                    />
                                </div>
                            </div>

                            {/* User 2 Section */}
                            <div className="space-y-4 border border-gray-200 p-4 rounded">
                                <h2 className="text-lg font-semibold text-gray-600">Partner 2</h2>
                                <div>
                                    <label htmlFor="user2-username" className="block text-sm font-medium text-gray-700">Username</label>
                                    <input
                                        type="text" id="user2-username" value={user2.username}
                                        onChange={(e) => handleInputChange(2, 'username', e.target.value)} required
                                        className="mt-1 block w-full input-style" autoComplete="new-username"
                                    />
                                </div>
                                <div>
                                    <label htmlFor="user2-firstname" className="block text-sm font-medium text-gray-700">First Name</label>
                                    <input
                                        type="text" id="user2-firstname" value={user2.first_name}
                                        onChange={(e) => handleInputChange(2, 'first_name', e.target.value)} required
                                        className="mt-1 block w-full input-style"
                                    />
                                </div>
                                <div>
                                    <label htmlFor="user2-password"className="block text-sm font-medium text-gray-700">Password</label>
                                    <input
                                        type="password" id="user2-password" value={user2.password}
                                        onChange={(e) => handleInputChange(2, 'password', e.target.value)} required minLength={6}
                                        className="mt-1 block w-full input-style" autoComplete="new-password"
                                    />
                                </div>
                            </div>
                        </div>

                        {/* Submit Button */}
                        <div className="pt-4"> {/* Add padding top */}
                            <button
                                type="submit"
                                disabled={isLoading || !!successMessage} // Disable after success
                                className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isLoading || successMessage ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
                            >
                                {isLoading ? 'Registering...' : 'Register Partners'}
                            </button>
                        </div>
                    </form>
                </div>
            </div>
            {/* Simple helper class for inputs */}
            <style>{`
                .input-style {
                    padding: 0.5rem 0.75rem;
                    border: 1px solid #d1d5db; /* gray-300 */
                    border-radius: 0.375rem; /* rounded-md */
                    box-shadow: 0 1px 2px 0 rgba(0, 0, 0, 0.05); /* shadow-sm */
                }
                .input-style:focus {
                    outline: none;
                    border-color: #6366f1; /* indigo-500 */
                    box-shadow: 0 0 0 1px #6366f1; /* ring-indigo-500 */
                }
            `}</style>
        </div>
    );
}

export default PartnerRegistrationForm;
