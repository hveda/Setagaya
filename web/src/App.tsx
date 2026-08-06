import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import DashboardLayout from './components/DashboardLayout';
import LiveStatus from './pages/LiveStatus';
import Reports from './pages/Reports';
import Reservations from './pages/Reservations';

export default function App() {
  return (
    <BrowserRouter>
      <DashboardLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/reports" replace />} />
          <Route path="/reports" element={<Reports />} />
          <Route path="/reports/:runId" element={<Reports />} />
          <Route path="/reservations" element={<Reservations />} />
          <Route path="/status" element={<LiveStatus />} />
        </Routes>
      </DashboardLayout>
    </BrowserRouter>
  );
}
