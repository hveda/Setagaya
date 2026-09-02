import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import DashboardLayout from './components/DashboardLayout';
import Clusters from './pages/Clusters';
import Campaigns from './pages/Campaigns';
import Executions from './pages/Executions';
import NewTest from './pages/NewTest';
import Execution from './pages/Execution';
import Reports from './pages/Reports';
import Reservations from './pages/Reservations';

export default function App() {
  return (
    <BrowserRouter>
      <DashboardLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/reports" replace />} />
          <Route path="/executions/new" element={<NewTest />} />
          <Route path="/executions" element={<Executions />} />
          <Route path="/executions/:id" element={<Execution />} />
          {/* R1: /status is now the execution hub's job; keep the bookmark alive. */}
          <Route path="/status" element={<Navigate to="/executions" replace />} />
          <Route path="/reports" element={<Reports />} />
          <Route path="/reports/:runId" element={<Reports />} />
          <Route path="/reservations" element={<Reservations />} />

          <Route path="/campaigns" element={<Campaigns />} />
          <Route path="/clusters" element={<Clusters />} />
        </Routes>
      </DashboardLayout>
    </BrowserRouter>
  );
}
