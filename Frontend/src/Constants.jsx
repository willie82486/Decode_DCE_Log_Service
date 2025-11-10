export const API_DECODE_URL = '/api/decode'; 
export const API_LOGIN_URL = '/api/login'; 
export const API_ADMIN_USERS_URL = '/api/admin/users'; 
export const API_ADMIN_PUSHTAGS_URL = '/api/admin/pushtags';

// --- Firebase Setup (not used) ---
export const firebaseConfig = null; 
export const appId = 'default-app-id'; 

// Helper component: Displays status messages
export const StatusMessage = ({ message, type }) => {
    if (!message) return null;
    const baseClasses = "mt-6 p-4 rounded-lg text-sm text-center";
    let typeClasses = '';
    if (type === 'error') typeClasses = 'bg-red-100 text-red-700';
    else if (type === 'success') typeClasses = 'bg-green-100 text-green-700';
    else if (type === 'warning') typeClasses = 'bg-yellow-100 text-yellow-700';
    else typeClasses = 'bg-blue-100 text-blue-700';

    return (
        <div className={`${baseClasses} ${typeClasses} whitespace-pre-wrap`}>
            {message}
        </div>
    );
};


