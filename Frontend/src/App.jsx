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
        <div className="w-full min-h-screen bg-gradient-to-br from-indigo-50 via-white to-sky-50 text-gray-800 py-10">
            <header className="max-w-5xl mx-auto flex justify-between items-center p-4 md:p-5 bg-white/70 backdrop-blur border border-white/60 shadow-md rounded-2xl mb-6">
                <h1 className="text-2xl md:text-3xl font-extrabold text-indigo-700 tracking-tight">DCE-FW Service</h1>
                
                {isLoggedIn && (
                    <div className="flex items-center space-x-3">
                        <nav className="flex space-x-2">
                            <button 
                                onClick={() => setCurrentView('decoder')}
                                className={`px-3 py-1 rounded-xl text-sm font-medium transition duration-150 ${currentView === 'decoder' ? 'bg-indigo-600 text-white shadow' : 'text-gray-700 hover:bg-gray-100'}`}
                            >
                                Log Decoder
                            </button>
                            
                            {userRole === 'admin' && ( 
                                <button 
                                    onClick={() => setCurrentView('admin')}
                                    className={`px-3 py-1 rounded-xl text-sm font-medium transition duration-150 ${currentView === 'admin' ? 'bg-red-600 text-white shadow' : 'text-gray-700 hover:bg-gray-100'}`}
                                >
                                    Admin
                                </button>
                            )}
                        </nav>
                        <button 
                            onClick={handleLogout}
                            className="px-3 py-1 rounded-xl text-sm font-medium text-white bg-gray-700 hover:bg-gray-800 transition duration-150"
                        >
                            Logout
                        </button>
                    </div>
                )}
            </header>
            
            <main className="max-w-5xl mx-auto px-4 md:px-6 pb-12">
                {isLoggedIn && <StatusMessage message={authStatus.message} type={authStatus.type} />}
                {renderContent()}
            </main>
        </div>
    );
};
export default App;


