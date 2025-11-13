import React, { useState, useCallback, useEffect } from 'react';
import LoginPage from './LoginPage.jsx';
import LogDecoder from './LogDecoder.jsx';
import AdminPage from './AdminPage.jsx';
import { StatusMessage } from './Constants.jsx'; 

const App = () => {
    const [isLoggedIn, setIsLoggedIn] = useState(false);
    const [currentView, setCurrentView] = useState('decoder'); 
    const [userId, setUserId] = useState("mock-user-dce-001"); 
    const [userRole, setUserRole] = useState(null);
    const [token, setToken] = useState('');
    const [authStatus, setAuthStatus] = useState({ message: 'Welcome to DCE-FW Log Service.', type: 'info' });

    const handleLoginSuccess = useCallback((role, jwtToken) => { 
        setIsLoggedIn(true);
        setUserRole(role);
        setToken(jwtToken || '');
        setAuthStatus({ message: 'Login successful. Navigating to decoder.', type: 'success' });
        setCurrentView('decoder');
        try {
            const next = { isLoggedIn: true, userRole: role, currentView: 'decoder', token: jwtToken || '' };
            localStorage.setItem('dce-auth', JSON.stringify(next));
        } catch {}
    }, []);
    
    const handleLogout = useCallback(() => {
        setIsLoggedIn(false);
        setUserRole(null); 
        setToken('');
        setCurrentView('decoder');
        setAuthStatus({ message: 'Logged out successfully.', type: 'info' });
        try { localStorage.removeItem('dce-auth'); } catch {}
    }, []); 

    // Restore login state on first mount
    useEffect(() => {
        try {
            const raw = localStorage.getItem('dce-auth');
            if (raw) {
                const saved = JSON.parse(raw);
                if (saved?.isLoggedIn) {
                    setIsLoggedIn(true);
                    setUserRole(saved.userRole || null);
                    setToken(saved.token || '');
                    setCurrentView(saved.currentView || 'decoder');
                    setAuthStatus({ message: 'Session restored.', type: 'success' });
                }
            }
        } catch {}
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Persist view/role changes while logged in
    useEffect(() => {
        try {
            const raw = localStorage.getItem('dce-auth');
            const saved = raw ? JSON.parse(raw) : {};
            const next = { ...saved, isLoggedIn, userRole, currentView, token };
            if (isLoggedIn) {
                localStorage.setItem('dce-auth', JSON.stringify(next));
            }
        } catch {}
    }, [isLoggedIn, userRole, currentView, token]);

    const renderContent = () => {
        if (!isLoggedIn) {
            return <LoginPage onLoginSuccess={handleLoginSuccess} />;
        }
                switch (currentView) {
            case 'decoder':
                        return <LogDecoder userId={userId} token={token} />;
            case 'admin':
                        return <AdminPage userId={userId} token={token} />; 
            default:
                        return <LogDecoder userId={userId} token={token} />;
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


