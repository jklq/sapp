import { useState, useEffect } from 'react';
import { getToken, storeToken, removeToken } from './api';
import { LoginResponse } from './types';
import LoginForm from './LoginForm'; // Import the new Login form
import LogSpendingForm from './LogSpendingForm'; // Import the refactored spending form

interface UserInfo {
  userId: number;
  firstName: string;
}

function App() {
  // Authentication state
  const [authToken, setAuthToken] = useState<string | null>(getToken());
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null); // Store user info
  const [isLoadingAuth, setIsLoadingAuth] = useState<boolean>(true); // Check initial auth status

  // Effect to check token validity or fetch user info on load if token exists
  useEffect(() => {
    const currentToken = getToken();
    if (currentToken) {
      // TODO: Optionally add an API call here to verify the token
      // and fetch user details if they aren't stored alongside the token.
      // For this demo, we assume the token is valid if present.
      // If login response is stored, parse it. For now, we only have the token.
      setAuthToken(currentToken);
      // We don't have user info from just the token, handleLoginSuccess sets it.
      // If you stored user info in localStorage, load it here.
    }
    setIsLoadingAuth(false); // Finished checking auth status
  }, []);

  const handleLoginSuccess = (data: LoginResponse) => {
    storeToken(data.token);
    setAuthToken(data.token);
    setUserInfo({ userId: data.user_id, firstName: data.first_name });
    // Optionally store user info in localStorage as well
    // localStorage.setItem('userInfo', JSON.stringify({ userId: data.user_id, firstName: data.first_name }));
  };

  const handleLogout = () => {
    removeToken();
    setAuthToken(null);
    setUserInfo(null);
    // Optionally remove user info from localStorage
    // localStorage.removeItem('userInfo');
  };

  if (isLoadingAuth) {
    // Optional: Show a loading spinner while checking auth
    return <div className="min-h-screen bg-gray-100 flex items-center justify-center">Loading...</div>;
  }

  return (
    <div className="min-h-screen bg-gray-100 flex flex-col items-center justify-center p-4">
      {authToken ? (
        // User is logged in
        <>
          <div className="w-full max-w-md mb-4 flex justify-between items-center">
             {/* Display user info if available */}
             {userInfo && <span className="text-gray-600 text-sm">Welcome, {userInfo.firstName}!</span>}
             {/* Simple Logout Button */}
            <button
              onClick={handleLogout}
              className="text-sm text-indigo-600 hover:text-indigo-800"
            >
              Logout
            </button>
          </div>
          {/* Render the main application form */}
          <LogSpendingForm />
        </>
      ) : (
        // User is not logged in, show Login Form
        <LoginForm onLoginSuccess={handleLoginSuccess} />
      )}
    </div>
  );
}

export default App;
