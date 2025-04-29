import { useState, useEffect } from 'react';
import { getToken, storeToken, removeToken } from './api';
import { LoginResponse } from './types';
import LoginForm from './LoginForm';
import LogSpendingForm from './LogSpendingForm';
import HistoryList from './HistoryList';
import TransferPage from './TransferPage';
import PartnerRegistrationForm from './PartnerRegistrationForm';
import AddDepositForm from './AddDepositForm';
import EditDepositPage from './EditDepositPage';
import StatsPage from './StatsPage'; // Import the StatsPage component

type View = 'login' | 'register' | 'logSpending' | 'addDeposit' | 'viewHistory' | 'transfer' | 'editDeposit' | 'stats'; // Add 'stats' to View type

interface UserInfo {
  userId: number;
  firstName: string;
}

function App() {
  // Authentication state
  const [authToken, setAuthToken] = useState<string | null>(getToken());
  const [userInfo, setUserInfo] = useState<UserInfo | null>(null);
  const [isLoadingAuth, setIsLoadingAuth] = useState<boolean>(true);

  // View state
  const [currentView, setCurrentView] = useState<View>('login');
  const [editingDepositId, setEditingDepositId] = useState<number | null>(null); // State to hold ID for edit view

  // Effect to check token validity or fetch user info on load
  useEffect(() => {
    const currentToken = getToken();
    if (currentToken) {
      // TODO: Optionally add an API call here to verify the token
      // and fetch user details if they aren't stored alongside the token.
      // For this demo, we assume the token is valid if present.
      setAuthToken(currentToken);
      // We don't have user info from just the token, handleLoginSuccess sets it.
      // If you stored user info in localStorage, load it here.
      // If token exists, assume logged in and go to default logged-in view
      setCurrentView('logSpending');
    } else {
      // No token, stay on login view (or potentially register view if navigated there)
    }
    setIsLoadingAuth(false); // Finished checking auth status
  }, []); // Run only once on initial load

  const handleLoginSuccess = (data: LoginResponse) => {
    storeToken(data.token);
    setAuthToken(data.token);
    setUserInfo({ userId: data.user_id, firstName: data.first_name });
    setCurrentView('logSpending'); // Go to main app view after login
    // Optionally store user info in localStorage as well
  };

  const handleLogout = () => {
    removeToken();
    setAuthToken(null);
    setUserInfo(null);
    setCurrentView('login'); // Go back to login view on logout
    // Optionally remove user info from localStorage
  };

  // Navigate to registration page
  const showRegistration = () => {
    setCurrentView('register');
  };

  // Navigate back to login page (e.g., from registration)
  const showLogin = () => {
    setCurrentView('login');
  };

  // Navigate to Edit Deposit page
  const showEditDeposit = (depositId: number) => {
    setEditingDepositId(depositId);
    setCurrentView('editDeposit');
  };


  // Determine which component to render based on auth and view state
  const renderContent = () => {
    if (isLoadingAuth) {
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
      
      <div className="w-full max-w-4xl p-4 flex flex-col items-center pb-20">

        {/* Render the selected view */}
        {currentView === 'logSpending' && <LogSpendingForm />}
        {currentView === 'addDeposit' && <AddDepositForm />}
        {currentView === 'viewHistory' && (
            <HistoryList
                onBack={() => setCurrentView('logSpending')}
                onNavigateToEditDeposit={showEditDeposit} // Pass navigation handler
            />
        )}
        {currentView === 'transfer' && <TransferPage onBack={() => setCurrentView('logSpending')} />}
        {currentView === 'editDeposit' && editingDepositId !== null && (
            <EditDepositPage
                depositId={editingDepositId}
                onBack={() => setCurrentView('viewHistory')} // Go back to history after edit/cancel
            />
        )}
        {currentView === 'stats' && <StatsPage onBack={() => setCurrentView('logSpending')} />} {/* Render StatsPage */}


        {/* Bottom Navigation Bar (Only show on main authenticated views) */}
        {['logSpending', 'addDeposit', 'viewHistory', 'transfer', 'stats'].includes(currentView) && ( // Add 'stats' to the list of views with the nav bar
          <header className="fixed bottom-0 left-0 right-0 w-full bg-white border-t border-gray-200 rounded-t-lg p-3 z-50 max-w-4xl mx-auto">
            <div className="flex justify-between items-center">
              {/* Left Group: Welcome Message (Desktop) + Navigation */}
              <div className="flex items-center md:gap-4">
              {/* Welcome Message (visible on larger screens) */}
              <div className="hidden md:block">
                {userInfo && <span className="text-gray-700 text-sm">Welcome, {userInfo.firstName}!</span>}
              </div>

              {/* Navigation Icons/Links - Centered on mobile, left-aligned on desktop */}
              <nav className="flex-grow flex justify-center md:justify-start space-x-1 md:space-x-2">
                {/* Spend */}
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
                <span className="text-xs md:text-sm">Spend</span>
              </button>

               {/* Deposit */}
               <button
                onClick={() => setCurrentView('addDeposit')}
                disabled={currentView === 'addDeposit'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'addDeposit'
                    ? 'bg-green-100 text-green-700' // Use green theme for deposit
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700'
                }`}
                aria-label="Add Deposit"
              >
                {/* Icon: Plus Circle */}
                 <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5 mb-1 md:mb-0 md:mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                   <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3m0 0v3m0-3h3m-3 0H9m12 0a9 9 0 11-18 0 9 9 0 0118 0z" />
                 </svg>
                <span className="text-xs md:text-sm">Deposit</span>
              </button>

              {/* View History */}
              <button
                onClick={() => setCurrentView('viewHistory')}
                disabled={currentView === 'viewHistory'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'viewHistory'
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

              {/* Stats Button */}
              <button
                onClick={() => setCurrentView('stats')}
                disabled={currentView === 'stats'}
                className={`flex flex-col md:flex-row items-center p-2 rounded-md transition-colors duration-150 ${
                  currentView === 'stats'
                    ? 'bg-indigo-100 text-indigo-700'
                    : 'text-gray-500 hover:bg-gray-100 hover:text-gray-700'
                }`}
                aria-label="View Stats"
              >
                {/* Icon: Chart Pie */}
                <svg xmlns="http://www.w3.org/2000/svg" className="h-5 w-5 mb-1 md:mb-0 md:mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M11 3.055A9.001 9.001 0 1020.945 13H11V3.055z" />
                  <path strokeLinecap="round" strokeLinejoin="round" d="M20.488 9H15V3.512A9.025 9.025 0 0120.488 9z" />
                </svg>
                <span className="text-xs md:text-sm">Stats</span>
              </button>
            </nav>
            </div>


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
              </button>
            </div>
          </header>
        )}
      </div>
    );
  };


  return (
    <div className="min-h-screen bg-gray-100 flex flex-col items-center">
      {renderContent()}
    </div>
  );
}

export default App;
