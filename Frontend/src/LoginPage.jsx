import React, { useState, useCallback } from 'react';
import { API_LOGIN_URL, StatusMessage } from './Constants.jsx'; 

const LoginPage = ({ onLoginSuccess }) => {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [status, setStatus] = useState({ message: '', type: 'info' });
    const [isLoading, setIsLoading] = useState(false);

    const handleLogin = useCallback(async (e) => {
        e.preventDefault();

        if (!username || !password) {
            setStatus({ message: "Please enter both username and password.", type: 'error' });
            return;
        }

        setIsLoading(true);
        setStatus({ message: "Attempting login...", type: 'info' });

        try {
            const response = await fetch(API_LOGIN_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password }),
            });
            
            const result = await response.json();

            if (response.ok && result.success) {
                const role = result.role || 'user'; 
                const token = result.token || '';
                setStatus({ message: `Login successful! Welcome, ${username} (${role}).`, type: 'success' });
                onLoginSuccess(role, token)
            } else {
                setStatus({ message: result.message || "Login failed. Invalid credentials or server error.", type: 'error' });
            }

        } catch (error) {
            console.error("Login API Error:", error);
            setStatus({ message: `Network or server error during login: ${error.message}. Ensure Go backend is running.`, type: 'error' });
        } finally {
            setIsLoading(false);
        }
    }, [username, password, onLoginSuccess]);

    return (
        <div className="min-h-[70vh] grid place-items-center px-4">
            <div className="w-full max-w-sm bg-white/80 backdrop-blur p-8 rounded-2xl shadow-xl border border-indigo-100">
                <h2 className="text-3xl font-extrabold text-indigo-800 text-center mb-6 tracking-tight">User Login</h2>
                <form onSubmit={handleLogin} className="space-y-4">
                
                    <div>
                        <label className="block text-sm font-medium text-gray-700 mb-1">Username</label>
                        <input type="text" placeholder="e.g., user1 or admin" required value={username} onChange={e => setUsername(e.target.value)}
                            className="w-full px-4 py-2 border border-gray-300 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition duration-150" />
                    </div>
                    
            
                    <div>
                        <label className="block text-sm font-medium text-gray-700 mb-1">Password</label>
                        <input type="password" placeholder="password" required value={password} onChange={e => setPassword(e.target.value)}
                            className="w-full px-4 py-2 border border-gray-300 rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 transition duration-150" />
                    </div>
 
                    <button type="submit" disabled={isLoading}
                        className="w-full py-2.5 px-4 rounded-xl shadow-md text-base font-semibold text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 transition duration-150 ease-in-out disabled:opacity-50">
                        {isLoading ? 'Verifying...' : 'Login'}
                    </button>
                </form>
                <StatusMessage message={status.message} type={status.type} />
            </div>
        </div>
    );
};

export default LoginPage;


