import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import DashboardLayout from './components/DashboardLayout';
import Card, { CardContent, CardHeader, CardTitle } from './components/ui/Card';
import Reports from './pages/Reports';

function Placeholder({ title }: { title: string }) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{title}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-body-sm">This page is not built yet.</p>
      </CardContent>
    </Card>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <DashboardLayout>
        <Routes>
          <Route path="/" element={<Navigate to="/reports" replace />} />
          <Route path="/reports" element={<Reports />} />
          <Route path="/reports/:runId" element={<Reports />} />
          <Route path="/reservations" element={<Placeholder title="Reservations" />} />
          <Route path="/status" element={<Placeholder title="Live Status" />} />
        </Routes>
      </DashboardLayout>
    </BrowserRouter>
  );
}
