import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { ExternalLink } from 'lucide-react';
import Button from '../components/ui/Button';
import Card, { CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import CopyButton from '../components/ui/CopyButton';
import Input from '../components/ui/Input';
import { ApiError } from '../api/client';
import { getRunReport, getShardConfig, getShardLog, listExecutionReports } from '../api/reports';
import type { Outcome, Report } from '../api/reports';
import { formatApmLink, loadApmTemplate, saveApmTemplate } from '../api/apm';

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

/** Engine badge: the engine kind that produced the run (jmeter, k6, ...) -- rendered only when the report carries one. */
function EngineBadge({ engine }: { engine: string }) {
  return (
    <span className="inline-flex items-center rounded-full bg-sky-100 px-2.5 py-0.5 text-xs font-medium text-sky-800 dark:bg-sky-900/30 dark:text-sky-300">
      {engine}
    </span>
  );
}

/**
 * Cluster badge: the load origin. Absent cluster means the deployment
 * default -- rendered as nothing, not as "default", so legacy rows stay
 * quiet.
 */
function ClusterBadge({ cluster }: { cluster: string }) {
  return (
    <span className="inline-flex items-center rounded-full bg-violet-100 px-2.5 py-0.5 text-xs font-medium text-violet-800 dark:bg-violet-900/30 dark:text-violet-300">
      {cluster}
    </span>
  );
}

/** Validates the shard viewer's input: shards are 0-indexed non-negative integers; anything else is null. */
export function parseShard(raw: string): number | null {
  if (raw.trim() === '') {
    return null;
  }
  const n = Number(raw);
  return Number.isInteger(n) && n >= 0 ? n : null;
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

      <ApmTemplateSettings />

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
                    <div className="flex flex-wrap items-center gap-3">
                      <OutcomeBadge outcome={r.outcome} />
                      <span className="text-body-sm font-medium text-slate-900 dark:text-white">Run #{r.run_id}</span>
                      <span className="text-caption text-slate-500 dark:text-slate-400">scenario {r.scenario_id}</span>
                      {r.engine && <EngineBadge engine={r.engine} />}
                      {r.cluster && <ClusterBadge cluster={r.cluster} />}
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

/**
 * The APM deep-link template, stored per browser (localStorage): when set,
 * each run's correlation id links into the operator's own APM. Client-side
 * only -- the server never sees it (same precedent as the theme toggle).
 */
function ApmTemplateSettings() {
  const [template, setTemplate] = useState<string>(() => loadApmTemplate());
  const [saved, setSaved] = useState(false);

  return (
    <Card>
      <CardHeader>
        <CardTitle>APM deep-link template</CardTitle>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-4 sm:flex-row sm:items-end"
          onSubmit={(e) => {
            e.preventDefault();
            saveApmTemplate(template);
            setSaved(true);
          }}
        >
          {/* text, not url: the {correlation_id} placeholder is not a valid URL character, so url validation would reject it. */}
          <Input
            label="URL template"
            type="text"
            value={template}
            onChange={(e) => {
              setTemplate(e.target.value);
              setSaved(false);
            }}
            placeholder="https://apm.example.com/trace/{correlation_id}"
            helperText="Optional. Put {correlation_id} where the trace id goes; run pages then link each correlation id into your APM. Saved in this browser only."
            fullWidth
          />
          <Button type="submit" variant="secondary">
            Save template
          </Button>
        </form>
        {saved && (
          <p className="text-caption mt-3 text-emerald-600 dark:text-emerald-400" role="status">
            Template saved.
          </p>
        )}
      </CardContent>
    </Card>
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

/**
 * One engine shard's durable objects: the compiled config exactly as the
 * run used it, and the captured log that outlives the pod. Scenario comes
 * from the report (prefilled); the operator only picks the shard number.
 */
function ShardObjects({ report }: { report: Report }) {
  const [shard, setShard] = useState('0');
  const [config, setConfig] = useState<string | null>(null);
  const [log, setLog] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = async (raw: string) => {
    const parsed = parseShard(raw);
    if (parsed === null) {
      setError('Shard must be a non-negative integer.');
      setConfig(null);
      setLog(null);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [cfg, shardLog] = await Promise.all([
        getShardConfig(report.run_id, report.scenario_id, parsed),
        getShardLog(report.run_id, report.scenario_id, parsed),
      ]);
      setConfig(cfg);
      setLog(shardLog);
    } catch (err) {
      setConfig(null);
      setLog(null);
      setError(err instanceof ApiError ? err.message : 'Failed to load shard objects.');
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Shard objects</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-caption text-slate-500 dark:text-slate-400">
          Run #{report.run_id} · scenario {report.scenario_id} — pick a shard to view the config it ran with and the log it captured.
        </p>
        <form
          className="flex items-end gap-3"
          onSubmit={(e) => {
            e.preventDefault();
            void load(shard);
          }}
        >
          <div className="w-32">
            <Input
              label="Shard"
              type="number"
              min={0}
              value={shard}
              onChange={(e) => setShard(e.target.value)}
            />
          </div>
          <Button type="submit" variant="secondary" disabled={loading}>
            {loading ? 'Loading…' : 'Load shard'}
          </Button>
        </form>
        {error && (
          <p className="text-sm text-red-600 dark:text-red-400" role="alert">
            {error}
          </p>
        )}
        {config !== null && (
          <div className="space-y-2">
            <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Config (as the run used it)</p>
            <pre className="max-h-96 overflow-auto rounded-lg bg-slate-900 p-4 font-mono text-xs leading-relaxed text-slate-100 dark:bg-slate-950">
              {config}
            </pre>
          </div>
        )}
        {log !== null && (
          <div className="space-y-2">
            <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Log (captured engine output)</p>
            <pre className="max-h-96 overflow-auto rounded-lg bg-slate-900 p-4 font-mono text-xs leading-relaxed text-slate-100 dark:bg-slate-950">
              {log}
            </pre>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ReportDetail({ runId }: { runId: string }) {
  const navigate = useNavigate();
  const [report, setReport] = useState<Report | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [apmTemplate] = useState<string>(() => loadApmTemplate());

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
              {report.engine && (
                <div>
                  <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Engine</p>
                  <p className="text-body-sm text-slate-900 dark:text-white">{report.engine}</p>
                </div>
              )}
              {report.cluster && (
                <div>
                  <p className="text-caption font-medium text-slate-500 dark:text-slate-400">Cluster</p>
                  <p className="text-body-sm text-slate-900 dark:text-white">{report.cluster}</p>
                </div>
              )}
            </CardContent>
          </Card>

          {report.correlation_id && (
            <Card>
              <CardHeader>
                <CardTitle>Correlation id</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-wrap items-center gap-3">
                  <code className="rounded-md bg-slate-100 px-2 py-1 font-mono text-body-sm break-all text-slate-900 dark:bg-slate-900 dark:text-slate-100">
                    {report.correlation_id}
                  </code>
                  <CopyButton value={report.correlation_id} label="Copy correlation id" />
                  {formatApmLink(apmTemplate, report.correlation_id) && (
                    <a
                      href={formatApmLink(apmTemplate, report.correlation_id) as string}
                      target="_blank"
                      rel="noreferrer"
                      className="inline-flex items-center gap-1 text-sm font-medium text-sky-600 hover:text-sky-700 dark:text-sky-400 dark:hover:text-sky-300"
                    >
                      Open in APM <ExternalLink aria-hidden className="h-3.5 w-3.5" />
                    </a>
                  )}
                </div>
                <p className="text-caption mt-2 text-slate-500 dark:text-slate-400">
                  The trace id this run&apos;s load carried (traceparent/baggage); paste it into your APM to see exactly this run&apos;s traffic.
                </p>
              </CardContent>
            </Card>
          )}

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
                  )                  )}
                </ul>
              )}
            </CardContent>
          </Card>

          <ShardObjects report={report} />
        </>
      )}
    </div>
  );
}
