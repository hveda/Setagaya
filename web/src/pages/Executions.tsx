import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import Card, { CardContent } from '../components/ui/Card';
import { ApiError } from '../api/client';
import { listExecutions, type ExecutionSummary } from '../api/executions';
import { useSession } from '../hooks/useSession';

/**
 * /executions -- the caller-scoped execution list (phase 19 R1, over G1's
 * GET /api/executions). Each row links to /executions/:id (the R2 hub).
 * Newest first is the server's contract, not this page's job.
 */
export default function Executions() {
  const { can } = useSession();
  const [executions, setExecutions] = useState<ExecutionSummary[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    listExecutions()
      .then((rows) => {
        if (alive) setExecutions(rows);
      })
      .catch((err: unknown) => {
        if (alive) setError(err instanceof ApiError ? err.message : 'failed to load executions');
      });
    return () => {
      alive = false;
    };
  }, []);

  if (error) {
    return (
      <div className="space-y-4">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Executions</h1>
        <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-slate-900 dark:text-slate-100">Executions</h1>
        {can('execution', 'create') && (
          <Link
            to="/executions/new"
            className="rounded text-sm font-medium text-sky-600 hover:underline focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-sky-500 dark:text-sky-400"
          >
            + New test
          </Link>
        )}
      </div>
      {executions === null ? (
        <p className="text-sm text-slate-500">Loading…</p>
      ) : executions.length === 0 ? (
        <Card>
          <CardContent>
            <p className="text-sm text-slate-500">No executions yet.</p>
          </CardContent>
        </Card>
      ) : (
        <ul className="divide-y divide-slate-200 dark:divide-slate-700">
          {executions.map((e) => (
            <li key={e.id}>
              <Link
                to={`/executions/${e.id}`}
                className="flex items-center justify-between rounded py-3 text-sm hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-sky-500 dark:hover:bg-slate-800/50 px-2"
              >
                <span className="font-medium text-slate-900 dark:text-slate-100">{e.name}</span>
                <span className="text-slate-500">
                  {e.engine ?? 'default engine'}
                  {e.cluster ? ` · ${e.cluster}` : ''}
                  {' · '}
                  {new Date(e.created_time).toLocaleString()}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
