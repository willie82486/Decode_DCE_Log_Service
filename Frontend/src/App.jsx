import React, { useState, useCallback } from 'react';
import LoginPage from './LoginPage.jsx';
import LogDecoder from './LogDecoder.jsx';
import AdminPage from './AdminPage.jsx';
import { StatusMessage } from './Constants.jsx'; 

const App = () => {
    const [isLoggedIn, setIsLoggedIn] = useState(false);
    const [currentView, setCurrentView] = useState('decoder'); 
    const [userId, setUserId] = useState("mock-user-dce-001"); 
    const [userRole, setUserRole] = useState(null);
    const [authStatus, setAuthStatus] = useState({ message: 'Welcome to DCE-FW Log Service.', type: 'info' });

    const handleLoginSuccess = useCallback((role) => { 
        setIsLoggedIn(true);
        setUserRole(role);
        setAuthStatus({ message: 'Login successful. Navigating to decoder.', type: 'success' });
        setCurrentView('decoder');
    }, []);
    
    const handleLogout = useCallback(() => {
        setIsLoggedIn(false);
        setUserRole(null); 
        setCurrentView('decoder');
        setAuthStatus({ message: 'Logged out successfully.', type: 'info' });
    }, []); 

    const renderContent = () => {
        if (!isLoggedIn) {
            return <LoginPage onLoginSuccess={handleLoginSuccess} />;
        }
        switch (currentView) {
            case 'decoder':
                return <LogDecoder userId={userId} />;
            case 'admin':
                return <AdminPage userId={userId} />; 
            default:
                return <LogDecoder userId={userId} />;
        }
    };

    return (
        <div className="w-full min-h-screen bg-gray-100 py-10">
            <header className="max-w-4xl mx-auto flex justify-between items-center p-4 bg-white shadow-md rounded-lg mb-6">
                <h1 className="text-2xl font-bold text-indigo-700">DCE-FW Service</h1>
                
                {isLoggedIn && (
                    <div className="flex items-center space-x-3">
                        <nav className="flex space-x-2">
                            <button 
                                onClick={() => setCurrentView('decoder')}
                                className={`px-3 py-1 rounded-lg text-sm font-medium transition duration-150 ${currentView === 'decoder' ? 'bg-indigo-500 text-white shadow' : 'text-gray-600 hover:bg-gray-100'}`}
                            >
                                Log Decoder
                            </button>
                            
                            {userRole === 'admin' && ( 
                                <button 
                                    onClick={() => setCurrentView('admin')}
                                    className={`px-3 py-1 rounded-lg text-sm font-medium transition duration-150 ${currentView === 'admin' ? 'bg-red-500 text-white shadow' : 'text-gray-600 hover:bg-gray-100'}`}
                                >
                                    Admin
                                </button>
                            )}
                        </nav>
                        <button 
                            onClick={handleLogout}
                            className="px-3 py-1 rounded-lg text-sm font-medium text-white bg-gray-500 hover:bg-gray-600 transition duration-150"
                        >
                            Logout
                        </button>
                    </div>
                )}
            </header>
            
            <main className="max-w-4xl mx-auto">
                {isLoggedIn && <StatusMessage message={authStatus.message} type={authStatus.type} />}
                {renderContent()}
            </main>
        </div>
    );
};
export default App;


