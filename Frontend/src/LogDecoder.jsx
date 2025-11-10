import React, { useState, useCallback } from 'react';
import { API_DECODE_URL, StatusMessage } from './Constants.jsx';

const LogDecoder = ({ userId }) => {
  const [pushtag, setPushtag] = useState('');
  const [buildId, setBuildId] = useState('');
  const [file, setFile] = useState(null);
  const [status, setStatus] = useState({ message: '', type: 'info' });
  const [isLoading, setIsLoading] = useState(false);

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault();
    if (!pushtag || !buildId || !file) {
      setStatus({ message: 'Please provide pushtag, buildId, and a file.', type: 'error' });
      return;
    }
    setIsLoading(true);
    setStatus({ message: 'Uploading and decoding ...', type: 'info' });
    try {
      const form = new FormData();
      form.append('pushtag', pushtag);
      form.append('buildId', buildId);
      form.append('file', file);
      const res = await fetch(API_DECODE_URL, { method: 'POST', body: form });
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
  }, [pushtag, buildId, file]);

  return (
    <div className="w-full max-w-2xl bg-white p-8 rounded-xl shadow-lg border border-gray-100 mx-auto">
      <h2 className="text-2xl font-bold text-indigo-700 mb-6">DCE Log Decoder</h2>
      <form onSubmit={handleSubmit} className="space-y-4">
        <div>
          <label className="block text-sm text-gray-700 mb-1">Pushtag</label>
          <input
            type="text" value={pushtag} onChange={e => setPushtag(e.target.value)} required
            className="w-full px-3 py-2 border rounded-lg focus:ring-indigo-500 focus:border-indigo-500"
          />
        </div>
        <div>
          <label className="block text-sm text-gray-700 mb-1">Build ID</label>
          <input
            type="text" value={buildId} onChange={e => setBuildId(e.target.value)} required
            className="w-full px-3 py-2 border rounded-lg focus:ring-indigo-500 focus:border-indigo-500"
          />
        </div>
        <div>
          <label className="block text-sm text-gray-700 mb-1">Encoded Log File (dce-enc.log)</label>
          <input type="file" accept=".log,.txt" onChange={e => setFile(e.target.files?.[0] || null)} required />
        </div>
        <button type="submit" disabled={isLoading} className="px-4 py-2 rounded-lg text-white bg-indigo-600 hover:bg-indigo-700 disabled:opacity-50">
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


