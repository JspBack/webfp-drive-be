import { Route, Routes } from 'react-router-dom';
import { Auth } from './pages/auth';
import { AuthCallback } from './pages/auth-callback';
import { Drive } from './pages/drive';

export default function App() {
  return (
    <Routes>
      <Route path="/" element={<Drive />} />
      <Route path="/f/:folderId" element={<Drive />} />
      <Route path="/auth/callback" element={<AuthCallback />} />
      <Route path="/auth/:pathname" element={<Auth />} />
    </Routes>
  );
}
