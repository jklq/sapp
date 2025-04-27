import { useState, useEffect } from 'react';
import { getToken, storeToken, removeToken } from './api';
import { LoginResponse } from './types';
import LoginForm from './LoginForm';
import LogSpendingForm from './LogSpendingForm';
import SpendingsList from './SpendingsList';
import TransferPage from './TransferPage';
import PartnerRegistrationForm from './PartnerRegistrationForm'; // Import the registration form

type View = 'login' | 'register' | 'logSpending' | 'viewSpendings' | 'transfer'; // Add 'register' view

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

  // View state (determines which component to show: login, register, or one of the authenticated views)
  const [currentView, setCurrentView] = useState<View>('login'); // Start at login by default

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
      // If token exists, assume logged in and go to default logged-in view
      setCurrentView('logSpending');
    } else {
      // No token, stay on login view (or potentially register view if navigated there)
      // setCurrentView('login'); // Already the default
    }
    setIsLoadingAuth(false); // Finished checking auth status
  }, []); // Run only once on initial load

  const handleLoginSuccess = (data: LoginResponse) => {
    storeToken(data.token);
    setAuthToken(data.token);
    setUserInfo({ userId: data.user_id, firstName: data.first_name });
    setCurrentView('logSpending'); // Go to main app view after login
    // Optionally store user info in localStorage as well
    // localStorage.setItem('userInfo', JSON.stringify({ userId: data.user_id, firstName: data.first_name }));
  };

  const handleLogout = () => {
    removeToken();
    setAuthToken(null);
    setUserInfo(null);
    setCurrentView('login'); // Go back to login view on logout
    // Optionally remove user info from localStorage
    // localStorage.removeItem('userInfo');
  };

  // Navigate to registration page
  const showRegistration = () => {
    setCurrentView('register');
  };

  // Navigate back to login page (e.g., from registration)
  const showLogin = () => {
    setCurrentView('login');
  };


  // Determine which component to render based on auth and view state
  const renderContent = () => {
    if (isLoadingAuth) {
      return <div className="min-h-screen bg-gray-100 flex items-center justify-center">Loading...</div>;
      return <div className="min-h-screen bg-gray-100 flex items-center justify-center">Loading...</div>;
    }

    // --- Unauthenticated Views ---
    if (!authToken) {
      if (currentView === 'register') {
        return <PartnerRegistrationForm onRegistrationSuccess={showLogin} onBackToLogin={showLogin} />;
      }
      // Default unauthenticated view is login
      return <LoginForm onLoginSuccess={handleLoginSuccess} onNavigateToRegister={showRegistration} />;
    }

    // --- Authenticated Views ---
    // User is logged in, show header and selected view
    return (
      // Use w-full and max-w-4xl for content consistency, add padding here
      // Make this a flex container to center the view component inside
      // Add padding-bottom to prevent content being hidden by fixed bottom nav (pb-20 is example, adjust as needed)
      <div className="w-full max-w-4xl p-4 flex flex-col items-center pb-20">

        {/* Render the selected view - these components now manage their own internal padding */}
        {/* The flex container above will center these components horizontally */}
        {currentView === 'logSpending' && <LogSpendingForm />}
        {currentView === 'viewSpendings' && <SpendingsList onBack={() => setCurrentView('logSpending')} />}
        {currentView === 'transfer' && <TransferPage onBack={() => setCurrentView('logSpending')} />}

        {/* Bottom Navigation Bar (Moved Header) */}
        {/* Fixed positioning, background, border-top, rounded-top */}
        <header className="fixed bottom-0 left-0 right-0 w-full bg-white border-t border-gray-200 rounded-t-lg p-3 z-50 max-w-4xl mx-auto">
          <div className="flex justify-between items-center">
            {/* Left Group: Welcome Message (Desktop) + Navigation */}
            <div className="flex items-center md:gap-4"> {/* Group welcome and nav */}
              {/* Welcome Message (visible on larger screens) */}
              <div className="hidden md:block">
                {userInfo && <span className="text-gray-700 text-sm">Welcome, {userInfo.firstName}!</span>}
              </div>

              {/* Navigation Icons/Links */}
              {/* Mobile: justify-around, w-full. Desktop: justify-start, w-auto */}
              <nav className="w-full md:w-auto flex justify-around md:justify-start space-x-1 md:space-x-1"> {/* Adjusted classes */}
                {/* Log Spending */}
                <button
                  onClick={() => setCurrentView('logSpending')}
                disabled={currentView === 'logSpending'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'logSpending'
                    ? 'bg-indigo-100 text-indigo-700'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700'
                }`}
                aria-label="Log Spending"
              >
                {/* Icon: Pencil */}
                <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5 mb-1 md:mb-0 md:mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                </svg>
                <span className="text-xs md:text-sm">Log</span>
              </button>

              {/* View History */}
              <button
                onClick={() => setCurrentView('viewSpendings')}
                disabled={currentView === 'viewSpendings'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'viewSpendings'
                    ? 'bg-indigo-100 text-indigo-700'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700'
                }`}
                aria-label="View History"
              >
                {/* Icon: List */}
                 <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5 mb-1 md:mb-0 md:mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01" />
                </svg>
                <span className="text-xs md:text-sm">History</span>
              </button>

              {/* Transfer Status */}
              <button
                onClick={() => setCurrentView('transfer')}
                disabled={currentView === 'transfer'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'transfer'
                    ? 'bg-indigo-100 text-indigo-700'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700'
                }`}
                aria-label="Transfer Status"
              >
                {/* Icon: Arrows */}
                <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5 mb-1 md:mb-0 md:mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                </svg>
                <span className="text-xs md:text-sm">Transfer</span>
              </button>
            </nav> {/* End Nav */}
            </div> {/* End Left Group */}


            {/* Logout Button (Remains on the right) */}
            <button
              onClick={handleLogout}
              className="text-sm text-red-600 hover:text-red-800 flex-shrink-0 p-2 rounded-md hover:bg-red-50"
              aria-label="Logout"
            >
               {/* Icon: Logout */}
               <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                 <path strokeLinecap="round" strokeLinejoin="round" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
               </svg>
               {/* Optional: Add text label for larger screens */}
               {/* <span className="hidden md:inline ml-1">Logout</span> */}
            </button>
          </div>
        </header> {/* End Bottom Nav Bar */}
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
