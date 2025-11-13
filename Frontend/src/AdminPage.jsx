import React, { useState, useCallback, useEffect, useRef } from 'react';
import { StatusMessage, API_ADMIN_USERS_URL, API_ADMIN_ELVES_URL, API_ADMIN_ELVES_UPLOAD_URL, API_ADMIN_ELVES_BY_URL_STREAM_URL, authHeader } from './Constants.jsx'; 

const AdminPage = ({ userId, token }) => {
    const [newUsername, setNewUsername] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [newRole, setNewRole] = useState('user');

    const [users, setUsers] = useState([]);
    const [status, setStatus] = useState({ message: '', type: 'info' });
    const [isLoading, setIsLoading] = useState(false);
    const [isFetching, setIsFetching] = useState(true); 

    const [pushtag, setPushtag] = useState('');
    const [pushtagUrl, setPushtagUrl] = useState('');
    const [elfs, setElfs] = useState([]);
    const [elfUploadFile, setElfUploadFile] = useState(null);
    const fileInputRef = useRef(null);
    const [byUrlSteps, setByUrlSteps] = useState([]);
    const streamAbortRef = useRef(null);
    const BY_URL_STATE_KEY = 'dce_by_url_state';

    const fetchUsers = useCallback(async () => {
        setIsFetching(true);
        setStatus({ message: "Fetching user list from backend...", type: 'info' });
        try {
            const response = await fetch(API_ADMIN_USERS_URL, { method: 'GET', headers: { 'Content-Type': 'application/json', ...authHeader(token) } });
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
    }, [token]);

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
                headers: { 'Content-Type': 'application/json', ...authHeader(token) },
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
    }, [newUsername, newPassword, newRole, fetchUsers, token]);

    const handleDeleteUser = useCallback(async (id, username) => {
        if (!id) return;
        const ok = window.confirm(`Delete user '${username}'?`);
        if (!ok) return;
        try {
            const response = await fetch(`${API_ADMIN_USERS_URL}?id=${encodeURIComponent(id)}`, { method: 'DELETE', headers: { ...authHeader(token) } });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${response.status}`);
            }
            setStatus({ message: `User '${username}' deleted.`, type: 'success' });
            fetchUsers();
        } catch (err) {
            setStatus({ message: `Failed to delete user: ${err.message}`, type: 'error' });
        }
    }, [fetchUsers, token]);

    const fetchElves = useCallback(async () => {
        try {
            const response = await fetch(API_ADMIN_ELVES_URL, { headers: { ...authHeader(token) } });
            const result = await response.json();
            if (!response.ok || !result.success) {
                throw new Error(result.message || `Failed to fetch elves: ${response.status}`);
            }
            setElfs(result.elves || []);
        } catch (err) {
            setElfs([]);
            setStatus({ message: `Error fetching elves: ${err.message}`, type: 'error' });
        }
    }, [token]);

    const handleFetchElfByURL = useCallback(async (e) => {
        e.preventDefault();
        if (!pushtag || !pushtagUrl) {
            setStatus({ message: "Pushtag and URL cannot be empty.", type: 'error' });
            return;
        }
        setIsLoading(true);
        try {
            // Use fetch to include Authorization header and manually parse SSE
            const initialSteps = ['Starting...'];
            setByUrlSteps(initialSteps);
            // Persist progress (prevent loss on page refresh F5)
            localStorage.setItem(BY_URL_STATE_KEY, JSON.stringify({
                pushtag,
                url: pushtagUrl,
                steps: initialSteps,
                status: 'running',
                startedAt: Date.now(),
            }));
            const controller = new AbortController();
            streamAbortRef.current = controller;
            const resp = await fetch(`${API_ADMIN_ELVES_BY_URL_STREAM_URL}?pushtag=${encodeURIComponent(pushtag)}&url=${encodeURIComponent(pushtagUrl)}`, {
                method: 'GET',
                headers: { ...authHeader(token), 'Accept': 'text/event-stream' },
                signal: controller.signal,
            });
            if (!resp.ok || !resp.body) {
                throw new Error(`Backend error: ${resp.status}`);
            }
            const reader = resp.body.getReader();
            const decoder = new TextDecoder();
            let buffer = '';
            const processChunk = (text) => {
                buffer += text;
                let idx;
                // Split SSE events by actual blank line delimiter
                while ((idx = buffer.indexOf('\n\n')) !== -1) {
                    const rawEvent = buffer.slice(0, idx);
                    buffer = buffer.slice(idx + 2);
                    // parse event block
                    let eventType = 'message';
                    let dataLines = [];
                    rawEvent.split('\n').forEach(line => {
                        if (line.startsWith('event:')) {
                            eventType = line.slice(6).trim();
                        } else if (line.startsWith('data:')) {
                            dataLines.push(line.slice(5).trim());
                        }
                    });
                    const data = dataLines.join('\\n');
                    if (eventType === 'step') {
                        setByUrlSteps(prev => {
                            const next = [...prev, data];
                            const saved = localStorage.getItem(BY_URL_STATE_KEY);
                            if (saved) {
                                try {
                                    const obj = JSON.parse(saved);
                                    obj.steps = next;
                                    obj.status = 'running';
                                    localStorage.setItem(BY_URL_STATE_KEY, JSON.stringify(obj));
                                } catch {}
                            }
                            return next;
                        });
                    } else if (eventType === 'error') {
                        setStatus({ message: `Fetch by URL failed: ${data}`, type: 'error' });
                        // Mark as failed and keep progress
                        const saved = localStorage.getItem(BY_URL_STATE_KEY);
                        if (saved) {
                            try {
                                const obj = JSON.parse(saved);
                                obj.steps = [...(obj.steps || []), `Error: ${data}`];
                                obj.status = 'error';
                                localStorage.setItem(BY_URL_STATE_KEY, JSON.stringify(obj));
                            } catch {}
                        }
                        controller.abort();
                        setIsLoading(false);
                    } else if (eventType === 'done') {
                        try { JSON.parse(data); } catch {}
                        setStatus({ message: `ELF fetched and stored.`, type: 'success' });
                        setPushtag(''); setPushtagUrl('');
                        fetchElves();
                        // Mark as done and keep final progress
                        const saved = localStorage.getItem(BY_URL_STATE_KEY);
                        if (saved) {
                            try {
                                const obj = JSON.parse(saved);
                                obj.steps = [...(obj.steps || []), 'Completed.'];
                                obj.status = 'done';
                                localStorage.setItem(BY_URL_STATE_KEY, JSON.stringify(obj));
                            } catch {}
                        }
                        controller.abort();
                        setIsLoading(false);
                    }
                }
            };
            while (true) {
                const { done, value } = await reader.read();
                if (done) break;
                processChunk(decoder.decode(value, { stream: true }));
            }
        } catch (err) {
            setStatus({ message: `Error fetching ELF by URL: ${err.message}`, type: 'error' });
            setIsLoading(false);
        }
    }, [pushtag, pushtagUrl, fetchElves, token]);

    // Restore "Fetch by URL" progress on mount (avoid loss after F5)
    useEffect(() => {
        try {
            const saved = localStorage.getItem(BY_URL_STATE_KEY);
            if (saved) {
                const obj = JSON.parse(saved);
                if (Array.isArray(obj.steps) && obj.steps.length > 0) {
                    setByUrlSteps(obj.steps);
                }
                // Do not auto-resume the request to avoid duplicate downloads; restore UI only
                // If needed in future, resume logic can be added here
            }
        } catch {}
    }, []);

    const handleUploadElf = useCallback(async (e) => {
        e.preventDefault();
        if (!elfUploadFile) {
            setStatus({ message: "Please choose an ELF file to upload.", type: 'error' });
            return;
        }
        setIsLoading(true);
        try {
            const form = new FormData();
            form.append('elf', elfUploadFile);
            const res = await fetch(API_ADMIN_ELVES_UPLOAD_URL, { method: 'POST', headers: { ...authHeader(token) }, body: form });
            const result = await res.json();
            if (!res.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${res.status}`);
            }
            setStatus({ message: `ELF uploaded. Build ID: ${result.buildId}`, type: 'success' });
            setElfUploadFile(null);
            if (fileInputRef.current) { fileInputRef.current.value = ''; }
            fetchElves();
        } catch (err) {
            setStatus({ message: `Error uploading ELF: ${err.message}`, type: 'error' });
        } finally {
            setIsLoading(false);
        }
    }, [elfUploadFile, fetchElves, token]);

    const handleDeleteElf = useCallback(async (buildId) => {
        if (!buildId) return;
        const ok = window.confirm(`Delete ELF record for buildId '${buildId}'?`);
        if (!ok) return;
        try {
            const res = await fetch(`${API_ADMIN_ELVES_URL}?buildId=${encodeURIComponent(buildId)}`, {
                method: 'DELETE',
                headers: { ...authHeader(token) },
            });
            const result = await res.json();
            if (!res.ok || !result.success) {
                throw new Error(result.message || `Backend error: ${res.status}`);
            }
            setStatus({ message: `ELF '${buildId}' deleted.`, type: 'success' });
            fetchElves();
        } catch (err) {
            setStatus({ message: `Failed to delete ELF: ${err.message}` , type: 'error' });
        }
    }, [token, fetchElves]);

    useEffect(() => {
        if (userId) {
            fetchUsers();
            fetchElves();
        } else {
            setStatus({ message: "User ID context is missing. Cannot load user list.", type: 'warning' });
        }
    }, [userId, fetchUsers, fetchElves]); 

    return (
        <div className="max-w-5xl mx-auto space-y-10">
            {/* User Management Card */}
            <div className="w-full max-w-2xl mx-auto bg-white/80 backdrop-blur p-8 rounded-2xl shadow-xl border border-white/60">
                <h2 className="text-2xl md:text-3xl font-extrabold text-indigo-700 mb-6 tracking-tight">IT Admin User Management</h2>
                <p className="mb-4 text-sm text-gray-600">
                    Manage users and pushtag mappings via the Go Backend API.
                </p>
                
                <form onSubmit={handleAddUser} className="space-y-4 p-4 border rounded-xl mb-6 bg-gray-50">
                    <h3 className="text-lg font-semibold text-gray-800">Add New User</h3>
                    <input 
                        type="text" placeholder="Username" required 
                        value={newUsername} onChange={e => setNewUsername(e.target.value)}
                        className="w-full px-3 py-2 border rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500" 
                    />
                    <input 
                        type="password" placeholder="Password (Plain Text)" required 
                        value={newPassword} onChange={e => setNewPassword(e.target.value)}
                        className="w-full px-3 py-2 border rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500" 
                    />
                    <div className="flex items-center gap-3">
                        <label className="text-sm text-gray-700">Role</label>
                        <select value={newRole} onChange={e => setNewRole(e.target.value)} className="px-3 py-2 border rounded-xl focus:outline-none">
                            <option value="user">user</option>
                            <option value="admin">admin</option>
                        </select>
                    </div>
                    <button type="submit" disabled={isLoading}
                        className="w-full py-2.5 px-4 rounded-xl text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 transition duration-150 disabled:opacity-50 shadow-md">
                        {isLoading ? 'Adding ...' : 'Add User'}
                    </button>
                </form>

                <StatusMessage message={status.message} type={status.type} />
                
                <h3 className="text-xl font-bold text-gray-800 mt-8 mb-4">Registered Users</h3>
                <div className="bg-white/80 border rounded-xl overflow-hidden shadow">
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
                                                className="px-3 py-1 text-xs rounded-lg bg-gray-100 hover:bg-red-100 text-red-700">
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
            </div>

            {/* ELF Library Management Card (wider) */}
            <div className="w-full bg-white/80 backdrop-blur p-8 rounded-2xl shadow-xl border border-white/60">
                <div className="flex items-end justify-between gap-4 mb-6">
                    <h3 className="text-2xl font-extrabold text-indigo-700 tracking-tight">ELF Library Management</h3>
                    <span className="text-xs text-gray-500">Manage build-id mapped ELF artifacts</span>
                </div>

                <form onSubmit={handleFetchElfByURL} className="space-y-4 p-4 border rounded-xl mb-6 bg-gray-50">
                    <h4 className="text-md font-semibold text-gray-800">Fetch ELF by URL</h4>
                    <div className="grid grid-cols-1 md:grid-cols-12 gap-3">
                        <input type="text" placeholder="pushtag" required value={pushtag} onChange={e => setPushtag(e.target.value)}
                            className="w-full px-3 py-2 border rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 md:col-span-3" />
                        <input type="url" placeholder="URL (e.g. http://.../pushtag/latest)" required value={pushtagUrl} onChange={e => setPushtagUrl(e.target.value)}
                            className="w-full px-3 py-2 border rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 md:col-span-7" />
                        <button type="submit" className="w-full md:w-auto md:col-span-2 px-4 py-2 rounded-xl text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 shadow-md">Fetch & Store</button>
                    </div>
                    {byUrlSteps.length > 0 && (
                        <div className="mt-3 p-3 bg-white rounded-lg border">
                            <div className="text-sm font-medium text-gray-700 mb-2">Progress</div>
                            <ul className="list-disc list-inside text-sm text-gray-700 space-y-1 max-h-60 overflow-auto">
                                {byUrlSteps.map((s, idx) => (<li key={idx}>{s}</li>))}
                            </ul>
                            <div className="mt-2 flex justify-end">
                                <button
                                    type="button"
                                    onClick={() => { setByUrlSteps([]); localStorage.removeItem(BY_URL_STATE_KEY); }}
                                    className="px-3 py-1 text-xs rounded-lg bg-gray-100 hover:bg-gray-200 text-gray-700"
                                >
                                    Clear
                                </button>
                            </div>
                        </div>
                    )}
                </form>

                <form onSubmit={handleUploadElf} className="space-y-4 p-4 border rounded-xl mb-6 bg-gray-50">
                    <h4 className="text-md font-semibold text-gray-800">Upload ELF</h4>
                    <div className="grid grid-cols-1 md:grid-cols-12 gap-3">
                        <input ref={fileInputRef} type="file" accept=".elf" onChange={e => setElfUploadFile(e.target.files?.[0] || null)}
                            className="w-full px-3 py-2 border rounded-xl focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 bg-white md:col-span-10" />
                        <div className="md:col-span-2 flex items-center">
                            <button type="submit" className="w-full px-4 py-2 rounded-xl text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 shadow-md disabled:opacity-50" disabled={isLoading}>Upload & Store</button>
                        </div>
                    </div>
                </form>

                <div className="bg-white/80 border rounded-xl overflow-hidden shadow">
                    <div className="overflow-x-auto">
                        <table className="min-w-full divide-y divide-gray-200">
                            <thead className="bg-gray-50">
                                <tr>
                                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">Build ID</th>
                                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">ELF File Name</th>
                                    <th className="px-6 py-3"></th>
                                </tr>
                            </thead>
                            <tbody className="bg-white divide-y divide-gray-200">
                                {elfs.length > 0 ? elfs.map((e) => (
                                    <tr key={e.buildId}>
                                        <td className="px-6 py-4 text-base font-mono text-gray-800 break-all">{e.buildId}</td>
                                        <td className="px-6 py-4 text-base text-gray-700 break-all">{e.elfFileName}</td>
                                        <td className="px-6 py-4 text-right">
                                            <button onClick={() => handleDeleteElf(e.buildId)} className="px-3 py-1 text-xs rounded-lg bg-gray-100 hover:bg-red-100 text-red-700">Delete</button>
                                        </td>
                                    </tr>
                                )) : (
                                    <tr>
                                        <td colSpan="3" className="px-6 py-4 text-sm text-gray-500">No ELF records yet.</td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    );
};

export default AdminPage;


