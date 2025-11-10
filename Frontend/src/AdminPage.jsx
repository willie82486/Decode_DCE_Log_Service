import React, { useState, useCallback, useEffect } from 'react';
import { StatusMessage, API_ADMIN_USERS_URL, API_ADMIN_PUSHTAGS_URL } from './Constants.jsx'; 

const AdminPage = ({ userId }) => {
    const [newUsername, setNewUsername] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [newRole, setNewRole] = useState('user');

    const [users, setUsers] = useState([]);
    const [status, setStatus] = useState({ message: '', type: 'info' });
    const [isLoading, setIsLoading] = useState(false);
    const [isFetching, setIsFetching] = useState(true); 

    const [pushtag, setPushtag] = useState('');
    const [pushtagUrl, setPushtagUrl] = useState('');
    const [pushtags, setPushtags] = useState([]);

    const fetchUsers = useCallback(async () => {
        setIsFetching(true);
        setStatus({ message: "Fetching user list from backend...", type: 'info' });
        try {
            const response = await fetch(API_ADMIN_USERS_URL, { method: 'GET', headers: { 'Content-Type': 'application/json' } });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Failed to fetch users: ${response.status}`);
            }
            setUsers(result.users || []);
            setStatus({ message: `Successfully loaded ${result.users.length} user(s).`, type: 'success' });
        } catch (error) {
            console.error("Error fetching users: ", error);
            setUsers([]);
            setStatus({ message: `Error fetching user list: ${error.message}`, type: 'error' });
        } finally {
            setIsFetching(false);
        }
    }, []);

    const handleAddUser = useCallback(async (e) => {
        e.preventDefault();
        if (!newUsername || !newPassword) {
            setStatus({ message: "Username and password cannot be empty.", type: 'error' });
            return;
        }
        setIsLoading(true);
        setStatus({ message: "Sending new user data to Go backend...", type: 'info' });
        try {
            const response = await fetch(API_ADMIN_USERS_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username: newUsername, password: newPassword, role: newRole }),
            });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${response.status}`);
            }
            setNewUsername('');
            setNewPassword('');
            setNewRole('user');
            setStatus({ message: result.message || `User '${newUsername}' added successfully.`, type: 'success' });
            fetchUsers();
        } catch (error) {
            console.error("Error adding user via API: ", error);
            setStatus({ message: `Error adding user: ${error.message}`, type: 'error' });
        } finally {
            setIsLoading(false);
        }
    }, [newUsername, newPassword, newRole, fetchUsers]);

    const handleDeleteUser = useCallback(async (id, username) => {
        if (!id) return;
        const ok = window.confirm(`Delete user '${username}'?`);
        if (!ok) return;
        try {
            const response = await fetch(`${API_ADMIN_USERS_URL}?id=${encodeURIComponent(id)}`, { method: 'DELETE' });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${response.status}`);
            }
            setStatus({ message: `User '${username}' deleted.`, type: 'success' });
            fetchUsers();
        } catch (err) {
            setStatus({ message: `Failed to delete user: ${err.message}`, type: 'error' });
        }
    }, [fetchUsers]);

    const fetchPushtags = useCallback(async () => {
        try {
            const response = await fetch(API_ADMIN_PUSHTAGS_URL);
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Failed to fetch pushtags: ${response.status}`);
            }
            setPushtags(result.pushtags || []);
        } catch (err) {
            setPushtags([]);
            setStatus({ message: `Error fetching pushtags: ${err.message}`, type: 'error' });
        }
    }, []);

    const handleAddPushtag = useCallback(async (e) => {
        e.preventDefault();
        if (!pushtag || !pushtagUrl) {
            setStatus({ message: "Pushtag and URL cannot be empty.", type: 'error' });
            return;
        }
        try {
            const response = await fetch(API_ADMIN_PUSHTAGS_URL, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ pushtag, url: pushtagUrl })
            });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${response.status}`);
            }
            setStatus({ message: `Pushtag '${pushtag}' saved.`, type: 'success' });
            setPushtag('');
            setPushtagUrl('');
            fetchPushtags();
        } catch (err) {
            setStatus({ message: `Error saving pushtag: ${err.message}`, type: 'error' });
        }
    }, [pushtag, pushtagUrl, fetchPushtags]);

    useEffect(() => {
        if (userId) {
            fetchUsers();
            fetchPushtags();
        } else {
            setStatus({ message: "User ID context is missing. Cannot load user list.", type: 'warning' });
        }
    }, [userId, fetchUsers, fetchPushtags]); 

    return (
        <div className="w-full max-w-2xl bg-white p-8 rounded-xl shadow-lg border border-gray-100 mx-auto">
            <h2 className="text-2xl font-bold text-red-700 mb-6">IT Admin User Management</h2>
            <p className="mb-4 text-sm text-gray-600">
                Manage users and pushtag mappings via the Go Backend API.
            </p>
            
            <form onSubmit={handleAddUser} className="space-y-4 p-4 border rounded-lg mb-6 bg-gray-50">
                <h3 className="text-lg font-semibold text-gray-800">Add New User</h3>
                <input 
                    type="text" placeholder="Username" required 
                    value={newUsername} onChange={e => setNewUsername(e.target.value)}
                    className="w-full px-3 py-2 border rounded-lg focus:ring-red-500 focus:border-red-500" 
                />
                <input 
                    type="password" placeholder="Password (Plain Text)" required 
                    value={newPassword} onChange={e => setNewPassword(e.target.value)}
                    className="w-full px-3 py-2 border rounded-lg focus:ring-red-500 focus:border-red-500" 
                />
                <div className="flex items-center gap-3">
                    <label className="text-sm text-gray-700">Role</label>
                    <select value={newRole} onChange={e => setNewRole(e.target.value)} className="px-3 py-2 border rounded-lg">
                        <option value="user">user</option>
                        <option value="admin">admin</option>
                    </select>
                </div>
                <button type="submit" disabled={isLoading}
                    className="w-full py-2 px-4 rounded-lg text-white bg-red-600 hover:bg-red-700 transition duration-150 disabled:opacity-50">
                    {isLoading ? 'Adding ...' : 'Add User'}
                </button>
            </form>

            <StatusMessage message={status.message} type={status.type} />
            
            <h3 className="text-xl font-bold text-gray-800 mt-8 mb-4">Registered Users</h3>
            <div className="bg-white border rounded-lg overflow-hidden shadow-sm">
                <table className="min-w-full divide-y divide-gray-200">
                    <thead className="bg-gray-50">
                        <tr>
                            <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Username</th>
                            <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Role</th>
                            <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">ID</th>
                            <th className="px-6 py-3"></th>
                        </tr>
                    </thead>
                    <tbody className="bg-white divide-y divide-gray-200">
                        {isFetching ? (
                             <tr>
                                <td colSpan="4" className="px-6 py-4 text-center text-sm text-blue-500">Loading user list from Go backend...</td>
                            </tr>
                        ) : users.length > 0 ? (
                            users.map((user) => (
                                <tr key={user.id}>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">{user.username}</td>
                                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">{user.role}</td>
                                    <td className="px-6 py-4 text-xs font-mono text-gray-400 truncate max-w-[100px]">{user.id}</td>
                                    <td className="px-6 py-4 text-right">
                                        <button onClick={() => handleDeleteUser(user.id, user.username)}
                                            className="px-3 py-1 text-xs rounded bg-gray-100 hover:bg-red-100 text-red-700">
                                            Delete
                                        </button>
                                    </td>
                                </tr>
                            ))
                        ) : (
                            <tr>
                                <td colSpan="4" className="px-6 py-4 text-center text-sm text-gray-500">No users found.</td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
            <p className="mt-4 text-xs text-gray-400 text-center">
                User ID used for auditing: <span className="font-mono text-gray-500 break-all">{userId}</span>
            </p>

            <hr className="my-8" />

            <h3 className="text-xl font-bold text-gray-800 mb-4">Pushtag Mapping</h3>
            <form onSubmit={handleAddPushtag} className="space-y-4 p-4 border rounded-lg mb-6 bg-gray-50">
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                    <input type="text" placeholder="pushtag" required value={pushtag} onChange={e => setPushtag(e.target.value)}
                        className="w-full px-3 py-2 border rounded-lg focus:ring-red-500 focus:border-red-500" />
                    <input type="url" placeholder="URL (e.g. http://.../pushtag/latest)" required value={pushtagUrl} onChange={e => setPushtagUrl(e.target.value)}
                        className="w-full px-3 py-2 border rounded-lg focus:ring-red-500 focus:border-red-500" />
                    <button type="submit" className="px-4 py-2 rounded-lg text-white bg-red-600 hover:bg-red-700">Save Mapping</button>
                </div>
            </form>

            <div className="bg-white border rounded-lg overflow-hidden shadow-sm">
                <table className="min-w-full divide-y divide-gray-200">
                    <thead className="bg-gray-50">
                        <tr>
                            <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Existing Pushtags</th>
                        </tr>
                    </thead>
                    <tbody className="bg-white divide-y divide-gray-200">
                        {pushtags.length > 0 ? pushtags.map((t) => (
                            <tr key={t}>
                                <td className="px-6 py-4 text-sm text-gray-800">{t}</td>
                            </tr>
                        )) : (
                            <tr><td className="px-6 py-4 text-sm text-gray-500">No pushtags yet.</td></tr>
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
};

export default AdminPage;


