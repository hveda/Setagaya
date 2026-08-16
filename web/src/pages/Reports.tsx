import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import Button from '../components/ui/Button';
import Card, { CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import Input from '../components/ui/Input';
import { ApiError } from '../api/client';
import { getRunReport, listExecutionReports } from '../api/reports';
import type { Outcome, Report } from '../api/reports';

const outcomeClasses: Record<Outcome, string> = {
  passed: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
  failed: 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300',
  aborted: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-300',
  error: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300',
};

function OutcomeBadge({ outcome }: { outcome: Outcome }) {
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${outcomeClasses[outcome]}`}>
      {outcome}
    </span>
  );
}

function formatTime(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
}

function sortedPercentiles(latency: Record<string, number>): [string, number][] {
  return Object.entries(latency).sort(([a], [b]) => Number(a) - Number(b));
}

/** The list view: an execution's reports, most recent first. */
export default function Reports() {
  const params = useParams();
  if (params.runId) {
    return <ReportDetail runId={params.runId} />;
  }
  return <ReportsList />;
}

function ReportsList() {
  const [executionId, setExecutionId] = useState('');
  const [reports, setReports] = useState<Report[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = async (id: string) => {
    const executionIdNum = Number(id);
    if (!id || !Number.isInteger(executionIdNum) || executionIdNum <= 0) {
      setError('Enter a valid execution id.');
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const got = await listExecutionReports(executionIdNum);
      setReports(got);
    } catch (err) {
      setReports(null);
      setError(err instanceof ApiError ? err.message : 'Failed to load reports.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-display-sm text-slate-900 dark:text-white">Reports</h1>
        <p className="text-body-sm mt-1 text-slate-500 dark:text-slate-400">
          An execution's run history, most recent first.
        </p>
      </div>

      <Card>
        <form
          className="flex flex-col gap-4 sm:flex-row sm:items-end"
          onSubmit={(e) => {
            e.preventDefault();
            void load(executionId);
          }}
        >
          <Input
            label="Execution ID"
            type="number"
            min={1}
            value={executionId}
            onChange={(e) => setExecutionId(e.target.value)}
            placeholder="e.g. 42"
            fullWidth
          />
          <Button type="submit" disabled={loading}>
            {loading ? 'Loading…' : 'Load reports'}
          </Button>
        </form>
        {error && (
          <p className="mt-4 text-sm text-red-600 dark:text-red-400" role="alert">
            {error}
          </p>
        )}
      </Card>

      {reports && (
        <Card padding="none">
          {reports.length === 0 ? (
            <p className="text-body-sm p-6 text-slate-500 dark:text-slate-400">No reports for this execution yet.</p>
          ) : (
            <ul className="divide-y divide-slate-200 dark:divide-slate-700">
              {reports.map((r) => (
                <li key={r.run_id}>
                  <Link
                    to={`/reports/${r.run_id}`}
                    className="flex min-h-[44px] flex-col gap-2 p-4 transition-colors hover:bg-slate-50 dark:hover:bg-slate-700/50 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="flex items-center gap-3">
                      <OutcomeBadge outcome={r.outcome} />
                      <span className="text-body-sm font-medium text-slate-900 dark:text-white">Run #{r.run_id}</span>
                      <span className="text-caption text-slate-500 dark:text-slate-400">scenario {r.scenario_id}</span>
                    </div>
                    <div className="text-caption text-slate-500 dark:text-slate-400">
                      {formatTime(r.started_at)} · {r.achieved.samples ?? 0} samples ·{' '}
                      {(r.error_rate * 100).toFixed(1)}% errors
                    </div>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </Card>
      )}
    </div>
  );
}

function LoadStat({ label, load }: { label: string; load: Report['requested'] }) {
  return (
    <div>
      <p className="text-caption font-medium text-slate-500 dark:text-slate-400">{label}</p>
      <p className="text-heading-md text-slate-900 dark:text-white">{load.concurrency} VU</p>
      <p className="text-caption text-slate-500 dark:text-slate-400">
        {load.throughput > 0 ? `${load.throughput.toFixed(1)} req/s target` : 'unlimited req/s'}
        {load.duration_seconds ? ` · ${load.duration_seconds}s` : ''}
      </p>
      {(load.samples !== undefined || load.failed !== undefined) && (
        <p className="text-caption text-slate-500 dark:text-slate-400">
          {load.samples ?? 0} samples, {load.failed ?? 0} failed
        </p>
      )}
    </div>
  );
}

function ReportDetail({ runId }: { runId: string }) {
  const navigate = useNavigate();
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const id = Number(runId);
    if (!Number.isInteger(id) || id <= 0) {
      setError('Invalid run id.');
      return;
    }
    setError(null);
    setReport(null);
    getRunReport(id)
      .then(setReport)
      .catch((err: unknown) => setError(err instanceof ApiError ? err.message : 'Failed to load report.'));
  }, [runId]);

  return (
    <div className="space-y-6">
      <Button variant="ghost" size="sm" onClick={() => navigate('/reports')}>
        ← Back to reports
      </Button>

      {error && (
        <Card>
          <p className="text-sm text-red-600 dark:text-red-400" role="alert">
            {error}
          </p>
        </Card>
      )}

      {report && (
        <>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <CardTitle>Run #{report.run_id}</CardTitle>
              <OutcomeBadge outcome={report.outcome} />
            </CardHeader>
            <CardContent className="grid grid-cols-2 gap-4 sm:grid-cols-4">
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Execution</p>
                <p className="text-body-sm text-slate-900 dark:text-white">{report.execution_id}</p>
              </div>
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Scenario</p>
                <p className="text-body-sm text-slate-900 dark:text-white">{report.scenario_id}</p>
              </div>
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Started</p>
                <p className="text-body-sm text-slate-900 dark:text-white">{formatTime(report.started_at)}</p>
              </div>
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Ended</p>
                <p className="text-body-sm text-slate-900 dark:text-white">{formatTime(report.ended_at)}</p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Load</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-1 gap-6 sm:grid-cols-2">
              <LoadStat label="Requested" load={report.requested} />
              <LoadStat label="Achieved" load={report.achieved} />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Latency percentiles</CardTitle>
            </CardHeader>
            <CardContent>
              {Object.keys(report.latency).length === 0 ? (
                <p className="text-body-sm">No latency data.</p>
              ) : (
                <div className="flex flex-wrap gap-4">
                  {sortedPercentiles(report.latency).map(([p, seconds]) => (
                    <div key={p} className="rounded-lg bg-slate-100 px-3 py-2 dark:bg-slate-700/50">
                      <p className="text-caption text-slate-500 dark:text-slate-400">p{p}</p>
                      <p className="text-body-sm font-semibold text-slate-900 dark:text-white">{seconds.toFixed(3)}s</p>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Attribution</CardTitle>
            </CardHeader>
            <CardContent className="grid grid-cols-3 gap-4">
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Target</p>
                <p className="text-heading-md text-slate-900 dark:text-white">{report.attribution.target}</p>
              </div>
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Engine</p>
                <p className="text-heading-md text-slate-900 dark:text-white">{report.attribution.engine}</p>
              </div>
              <div>
                <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Unknown</p>
                <p className="text-heading-md text-slate-900 dark:text-white">{report.attribution.unknown}</p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Error signatures</CardTitle>
            </CardHeader>
            <CardContent>
              {!report.errors || report.errors.length === 0 ? (
                <p className="text-body-sm">No failures recorded.</p>
              ) : (
                <ul className="space-y-3">
                  {report.errors.map((e, i) => (
                    <li key={i} className="rounded-lg border border-slate-200 p-3 dark:border-slate-700">
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-body-sm font-medium text-slate-900 dark:text-white">{e.label}</span>
                        <span className="text-caption text-slate-500 dark:text-slate-400">
                          {e.side} · {e.count} {e.count === 1 ? 'occurrence' : 'occurrences'}
                          {e.response_code ? ` · ${e.response_code}` : ''}
                        </span>
                      </div>
                      {e.exemplars && e.exemplars.length > 0 && (
                        <p className="text-caption mt-1 text-slate-500 dark:text-slate-400">{e.exemplars[0]}</p>
                      )}
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  );
}
