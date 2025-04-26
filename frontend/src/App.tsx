import { useState, useEffect } from 'react';
import { getToken, storeToken, removeToken } from './api';
import { LoginResponse } from './types';
import LoginForm from './LoginForm';
import LogSpendingForm from './LogSpendingForm';
import SpendingsList from './SpendingsList';
import TransferPage from './TransferPage'; // Import the new TransferPage component

type View = 'login' | 'logSpending' | 'viewSpendings' | 'transfer'; // Add 'transfer' view

interface UserInfo {
  userId: number;
  firstName: string;
}

function App() {
  // Authentication state
  // Authentication state
  const [authToken, setAuthToken] = useState<string | null>(getToken());
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null);
  const [isLoadingAuth, setIsLoadingAuth] = useState<boolean>(true);

  // View state (determines which component to show after login)
  const [currentView, setCurrentView] = useState<View>('logSpending'); // Default view after login

  // Effect to check token validity or fetch user info on load
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
    setCurrentView('logSpending'); // Reset view on logout
  };

  // Determine which component to render based on auth and view state
  const renderContent = () => {
    if (isLoadingAuth) {
      return <div className="min-h-screen bg-gray-100 flex items-center justify-center">Loading...</div>;
    }

    if (!authToken) {
      return <LoginForm onLoginSuccess={handleLoginSuccess} />;
    }

    // User is logged in, show header and selected view
    return (
      // Use w-full and max-w-4xl for content consistency, add padding here
      <div className="w-full max-w-4xl p-4">
        {/* Header: Make flex wrap and add spacing for mobile */}
        <div className="mb-4 flex flex-wrap justify-between items-center gap-2">
          {/* Left side: Welcome message and navigation */}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            {userInfo && <span className="text-gray-600 text-sm">Welcome, {userInfo.firstName}!</span>}
            {/* Navigation Links/Buttons */}
            <button
              onClick={() => setCurrentView('logSpending')}
              className={`text-sm mr-3 ${currentView === 'logSpending' ? 'text-indigo-700 font-semibold' : 'text-indigo-600 hover:text-indigo-800'}`}
              disabled={currentView === 'logSpending'}
            >
              Log Spending
            </button>
            <button
              onClick={() => setCurrentView('viewSpendings')}
              className={`text-sm ${currentView === 'viewSpendings' ? 'text-indigo-700 font-semibold' : 'text-indigo-600 hover:text-indigo-800'}`}
              disabled={currentView === 'viewSpendings'}
            >
              View History
            </button>
            <button
              onClick={() => setCurrentView('transfer')}
              className={`text-sm ${currentView === 'transfer' ? 'text-indigo-700 font-semibold' : 'text-indigo-600 hover:text-indigo-800'}`}
              disabled={currentView === 'transfer'}
            >
              Transfer Status
            </button>
          </div>
          {/* Right side: Logout button */}
          <button
            onClick={handleLogout}
            className="text-sm text-red-600 hover:text-red-800 flex-shrink-0"
          >
            Logout
          </button>
        </div>

        {/* Render the selected view - these components now manage their own internal padding */}
        {currentView === 'logSpending' && <LogSpendingForm />}
        {currentView === 'viewSpendings' && <SpendingsList onBack={() => setCurrentView('logSpending')} />}
        {currentView === 'transfer' && <TransferPage onBack={() => setCurrentView('logSpending')} />}
      </div>
    );
  };


  return (
    // Remove overall padding (pt-8 px-4), add padding within content sections as needed
    <div className="min-h-screen bg-gray-100 flex flex-col items-center">
      {renderContent()}
    </div>
  );
}

export default App;
