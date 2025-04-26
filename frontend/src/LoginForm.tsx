import { useState, FormEvent } from 'react';
import { loginUser } from './api';
import { LoginResponse } from './types';

interface LoginFormProps {
  onLoginSuccess: (data: LoginResponse) => void;
  onNavigateToRegister: () => void; // Callback to switch view to registration
}

function LoginForm({ onLoginSuccess, onNavigateToRegister }: LoginFormProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [isDemoLoading, setIsDemoLoading] = useState(false); // Separate loading state for demo button
  const [error, setError] = useState<string | null>(null);

  // Handler for the regular login form submission
  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    setIsLoading(true);

    if (!username || !password) {
      setError('Username and password are required.');
      setIsLoading(false);
      return;
    }

    try {
      const loginData = await loginUser({ username, password });
      onLoginSuccess(loginData); // Pass token and user info up to App
      // No need to reset form here, as the component will unmount/be replaced
    } catch (err) {
      console.error("Login failed:", err);
      setError(err instanceof Error ? err.message : 'An unknown login error occurred.');
      setIsLoading(false);
    }
    // No finally block needed for setIsLoading(false) because on success,
    // the component might unmount before it runs. It's set in the catch block.
  };

  // Handler for the "Login as Demo" button
  const handleDemoLogin = async () => {
    setError(null);
    setIsDemoLoading(true);
    try {
      // Use hardcoded demo credentials
      const loginData = await loginUser({ username: 'demo_user', password: 'password' });
      onLoginSuccess(loginData);
    } catch (err) {
      console.error("Demo login failed:", err);
      setError(err instanceof Error ? err.message : 'An unknown demo login error occurred.');
      setIsDemoLoading(false); // Ensure loading state is reset on error
    }
    // No finally block needed for setIsDemoLoading(false) for the same reason as above.
  };


  return (
    // Keep centering container, remove its padding
    <div className="min-h-screen bg-gray-100 flex items-center justify-center">
      {/* Remove p-6 from card, add p-4 inside */}
      <div className="bg-white shadow-md rounded-lg w-full max-w-sm">
        <div className="p-4"> {/* Add inner padding */}
          <h1 className="text-2xl font-bold mb-6 text-center text-gray-700">Login</h1>
          {error && <div className="bg-red-100 border border-red-400 text-red-700 px-4 py-3 rounded relative mb-4" role="alert">{error}</div>}
          <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="username" className="block text-sm font-medium text-gray-700">Username</label>
            <input
              type="text"
              id="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              required
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
              autoComplete="username"
            />
          </div>
          <div>
            <label htmlFor="password"className="block text-sm font-medium text-gray-700">Password</label>
            <input
              type="password"
              id="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
              className="mt-1 block w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 sm:text-sm"
              autoComplete="current-password"
            />
          </div>
          <div>
            <button
              type="submit"
              disabled={isLoading}
              className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isLoading ? 'bg-indigo-300 cursor-not-allowed' : 'bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500'}`}
            >
              {isLoading ? 'Logging in...' : 'Login'}
            </button>
          </div>
        </form>

        {/* Divider */}
        <div className="my-6 flex items-center justify-center">
          <span className="px-2 bg-white text-sm text-gray-500">OR</span>
        </div>

        {/* Demo Login Button */}
        <div>
          <button
            type="button" // Important: type="button" to prevent form submission
            onClick={handleDemoLogin}
            disabled={isDemoLoading || isLoading} // Disable if either login is in progress
            className={`w-full flex justify-center py-2 px-4 border border-transparent rounded-md shadow-sm text-sm font-medium text-white ${isDemoLoading || isLoading ? 'bg-gray-400 cursor-not-allowed' : 'bg-gray-600 hover:bg-gray-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-gray-500'}`}
          >
            {isDemoLoading ? 'Logging in as Demo...' : 'Login as Demo User'}
          </button>
        </div>

        {/* Link to Registration */}
        <div className="mt-4 text-center text-sm">
            <span className="text-gray-600">Don't have an account? </span>
            <button
                type="button"
                onClick={onNavigateToRegister}
                className="font-medium text-indigo-600 hover:text-indigo-500"
            >
                Register Partners
            </button>
        </div>

        </div> {/* Close inner padding div */}
      </div>
    </div>
  );
}

export default LoginForm;
