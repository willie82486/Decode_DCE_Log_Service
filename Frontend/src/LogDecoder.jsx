import React, { useState, useCallback } from 'react';
import { API_DECODE_URL, StatusMessage, authHeader } from './Constants.jsx';

const LogDecoder = ({ userId, token }) => {
  const [file, setFile] = useState(null);
  const [status, setStatus] = useState({ message: '', type: 'info' });
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    if (!file) {
      setStatus({ message: 'Please choose a dce-enc.log file.', type: 'error' });
      return;
    }
    setIsLoading(true);
    setStatus({ message: 'Uploading and decoding ...', type: 'info' });
    try {
      const form = new FormData();
      form.append('file', file);
      const res = await fetch(API_DECODE_URL, { method: 'POST', headers: { ...authHeader(token) }, body: form });
      if (!res.ok) {
        const text = await res.text();
        throw new Error(text || `Decode failed with ${res.status}`);
      }
      const blob = await res.blob();
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'dce-decoded.log';
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
      setStatus({ message: 'Decoded file downloaded as dce-decoded.log', type: 'success' });
    } catch (err) {
      setStatus({ message: `Decode error: ${err.message}`, type: 'error' });
    } finally {
      setIsLoading(false);
    }
  }, [file, token]);

  return (
    <div className="w-full max-w-2xl bg-white/80 backdrop-blur p-8 rounded-2xl shadow-xl border border-white/60 mx-auto">
      <h2 className="text-2xl md:text-3xl font-extrabold text-indigo-700 mb-6 tracking-tight">DCE Log Decoder</h2>
      <form onSubmit={handleSubmit} className="space-y-5">
        <div>
          <label className="block text-sm text-gray-700 mb-1">Encoded Log File (dce-enc.log)</label>
          <input
            type="file"
            accept=".log,.txt"
            onChange={e => setFile(e.target.files?.[0] || null)}
            required
            className="block w-full text-sm text-gray-700
                       file:mr-4 file:py-2 file:px-4
                       file:rounded-lg file:border-0
                       file:bg-indigo-50 file:text-indigo-700
                       hover:file:bg-indigo-100"
          />
        </div>
        <button
          type="submit"
          disabled={isLoading}
          className="px-5 py-2.5 rounded-xl text-white bg-gradient-to-r from-indigo-600 to-blue-600 hover:from-indigo-700 hover:to-blue-700 disabled:opacity-50 shadow-md"
        >
          {isLoading ? 'Decoding...' : 'Decode and Download'}
        </button>
      </form>
      <StatusMessage message={status.message} type={status.type} />
      <p className="mt-4 text-xs text-gray-400 text-center">
        Operator ID: <span className="font-mono text-gray-500 break-all">{userId}</span>
      </p>
    </div>
  );
};

export default LogDecoder;


