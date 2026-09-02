import { useEffect, useRef, useState } from 'react';
import { useParams } from 'react-router-dom';
import Button from '../components/ui/Button';
import Card, { CardContent, CardHeader, CardTitle } from '../components/ui/Card';
import { ApiError } from '../api/client';
import { getExecutionInfo, getExecutionStatus, streamExecutionMetrics } from '../api/status';
import type { EngineMetric, ExecutionInfo, ExecutionStatus, Phase, ScenarioStatus } from '../api/status';
import { deployExecution, purgeExecution, stopExecution, triggerExecution } from '../api/lifecycle';
import ClusterBadge from '../components/ui/ClusterBadge';
import EngineBadge from '../components/ui/EngineBadge';

const phaseClasses: Record<Phase, string> = {
  idle: 'bg-slate-200 text-slate-700 dark:bg-slate-700 dark:text-slate-300',
  deployed: 'bg-sky-100 text-sky-800 dark:bg-sky-900/30 dark:text-sky-300',
  running: 'bg-emerald-100 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300',
};

function PhaseBadge({ phase }: { phase: Phase }) {
  return (
    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${phaseClasses[phase]}`}>
      {phase}
    </span>
  );
}

/**
 * The stream carries one event per measured interval (roughly once a second
 * per active label/shard), not one per request -- there is no per-request
 * count to work with here, only what the interval already summarised. A
 * trailing window over received events is the closest "current" signal
 * obtainable without adding a new backend aggregation.
 */
const windowMs = 10_000;

interface ReceivedMetric {
  receivedAt: number;
  metric: EngineMetric;
}

interface LiveStats {
  throughput: number;
  errorRate: number;
  latencySeconds: number | null;
}

function summarize(events: ReceivedMetric[]): LiveStats {
  if (events.length === 0) {
    return { throughput: 0, errorRate: 0, latencySeconds: null };
  }
  const errors = events.filter((e) => e.metric.status !== '200').length;
  return {
    throughput: events.length / (windowMs / 1000),
    errorRate: errors / events.length,
    latencySeconds: events[events.length - 1].metric.latency,
  };
}

/**
 * Engines still missing for a scenario: wanted minus deployed, floored at
 * zero (a terminating engine can briefly report one extra). Drives the
 * pending-engines mark on the scenario rows.
 */
export function engineShortfall(s: Pick<ScenarioStatus, 'engines' | 'engines_deployed'>): number {
  return Math.max(0, s.engines - s.engines_deployed);
}

function StatCard({ label, value, caption }: { label: string; value: string; caption: string }) {
  return (
    <div>
      <p className="text-caption font-medium text-slate-500 dark:text-slate-400">{label}</p>
      <p className="text-heading-md text-slate-900 dark:text-white">{value}</p>
      <p className="text-caption text-slate-500 dark:text-slate-400">{caption}</p>
    </div>
  );
}

/**
 * Which lifecycle actions the hub offers, per phase and per engine
 * reachability. The matrix is the R2 contract: deploy when idle, trigger
 * when deployed (and engines reachable), stop while running, purge whenever
 * something is deployed or running. Disabled buttons say WHY.
 */
export function phaseControls(phase: Phase | null, enginesReachable: boolean): Array<{ action: 'deploy' | 'trigger' | 'stop' | 'purge'; enabled: boolean }> {
  switch (phase) {
    case 'idle':
      return [
        { action: 'deploy', enabled: true },
        { action: 'trigger', enabled: false },
        { action: 'stop', enabled: false },
        { action: 'purge', enabled: false },
      ];
    case 'deployed':
      return [
        { action: 'deploy', enabled: false },
        { action: 'trigger', enabled: enginesReachable },
        { action: 'stop', enabled: false },
        { action: 'purge', enabled: true },
      ];
    case 'running':
      return [
        { action: 'deploy', enabled: false },
        { action: 'trigger', enabled: false },
        { action: 'stop', enabled: true },
        { action: 'purge', enabled: true },
      ];
    default:
      // Status not loaded yet: nothing to click.
      return [
        { action: 'deploy', enabled: false },
        { action: 'trigger', enabled: false },
        { action: 'stop', enabled: false },
        { action: 'purge', enabled: false },
      ];
  }
}

/** A watched execution's live phase, deployment status, rolling metrics, and lifecycle controls. Deep-linkable: the execution id comes from the route. */
export default function Execution() {
  const { id } = useParams<{ id: string }>();
  const executionId = Number(id);
  const validId = Number.isInteger(executionId) && executionId > 0;

  const [info, setInfo] = useState<ExecutionInfo | null>(null);
  const [status, setStatus] = useState<ExecutionStatus | null>(null);
  const [stats, setStats] = useState<LiveStats>({ throughput: 0, errorRate: 0, latencySeconds: null });
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [busyAction, setBusyAction] = useState<string | null>(null);
  const eventsRef = useRef<ReceivedMetric[]>([]);

  useEffect(() => {
    if (!validId) {
      return;
    }
    let cancelled = false;
    // The execution's setup (engine kind, cluster) is immutable -- fetched
    // once. The lifecycle snapshot changes as engines deploy, so it polls.
    getExecutionInfo(executionId)
      .then((i) => {
        if (!cancelled) setInfo(i);
      })
      .catch((err: unknown) => {
        if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load execution.');
      });
    const loadStatus = () => {
      getExecutionStatus(executionId)
        .then((s) => {
          if (!cancelled) setStatus(s);
        })
        .catch((err: unknown) => {
          if (!cancelled) setError(err instanceof ApiError ? err.message : 'Failed to load status.');
        });
    };
    loadStatus();
    const poll = setInterval(loadStatus, 10_000);
    eventsRef.current = [];
    setStats({ throughput: 0, errorRate: 0, latencySeconds: null });
    const prune = () => {
      const now = Date.now();
      eventsRef.current = eventsRef.current.filter((e) => now - e.receivedAt < windowMs);
      setStats(summarize(eventsRef.current));
    };
    const unsubscribe = streamExecutionMetrics(executionId, (metric) => {
      eventsRef.current = [...eventsRef.current, { receivedAt: Date.now(), metric }];
      prune();
    });
    const ticker = setInterval(prune, 1000);
    return () => {
      cancelled = true;
      unsubscribe();
      clearInterval(ticker);
      clearInterval(poll);
    };
  }, [executionId, validId]);

  const runAction = (action: 'deploy' | 'trigger' | 'stop' | 'purge') => {
    setBusyAction(action);
    setActionError(null);
    const fn = { deploy: deployExecution, trigger: triggerExecution, stop: stopExecution, purge: purgeExecution }[action];
    fn(executionId)
      .then((message) => {
        setActionError(null);
        // The mutation succeeded; refresh the snapshot immediately rather
        // than waiting for the next poll tick.
        getExecutionStatus(executionId).then((s) => setStatus(s));
        if (action === 'purge') {
          // Purged: the hub's live view resets to the idle snapshot.
          setStats({ throughput: 0, errorRate: 0, latencySeconds: null });
        }
        void message;
      })
      .catch((err: unknown) => {
        // A 409 surfaces the server's message verbatim -- that is the whole
        // point of the typed ApiError, not a generic failure string.
        setActionError(err instanceof ApiError ? err.message : `${action} failed.`);
      })
      .finally(() => setBusyAction(null));
  };

  if (!validId) {
    return (
      <div className="space-y-4">
        <h1 className="text-display-sm text-slate-900 dark:text-white">Execution</h1>
        <p className="text-body-sm text-red-600 dark:text-red-400">Invalid execution id.</p>
      </div>
    );
  }

  const enginesReachable = status?.status.every((s) => s.engines_reachable) ?? false;
  const controls = phaseControls(status?.phase ?? null, enginesReachable);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-display-sm text-slate-900 dark:text-white">Execution #{executionId}</h1>
        <p className="text-body-sm mt-1 text-slate-500 dark:text-slate-400">
          Deployment status, lifecycle controls, and rolling metrics.
        </p>
      </div>
      {error && (
        <p className="text-sm text-red-600 dark:text-red-400" role="alert">
          {error}
        </p>
      )}
      {status && (
        <>
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div className="flex flex-wrap items-center gap-2">
                <CardTitle>Execution #{executionId}</CardTitle>
                {info?.engine && <EngineBadge engine={info.engine} />}
                {info?.cluster && <ClusterBadge cluster={info.cluster} />}
              </div>
              <PhaseBadge phase={status.phase} />
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid grid-cols-1 gap-6 sm:grid-cols-3">
                <StatCard
                  label="Throughput"
                  value={`${stats.throughput.toFixed(1)}/s`}
                  caption="samples/sec, trailing 10s"
                />
                <StatCard
                  label="Error rate"
                  value={`${(stats.errorRate * 100).toFixed(1)}%`}
                  caption="trailing 10s"
                />
                <StatCard
                  label="Latency (p50)"
                  value={stats.latencySeconds !== null ? `${(stats.latencySeconds * 1000).toFixed(0)} ms` : '—'}
                  caption="most recent sample"
                />
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {controls.map(({ action, enabled }) => (
                  <Button
                    key={action}
                    variant={action === 'stop' || action === 'purge' ? 'outline' : 'primary'}
                    disabled={!enabled || busyAction !== null}
                    onClick={() => runAction(action)}
                  >
                    {busyAction === action ? 'Working…' : action.charAt(0).toUpperCase() + action.slice(1)}
                  </Button>
                ))}
              </div>
              {actionError && (
                <p className="text-sm text-red-600 dark:text-red-400" role="alert">
                  {actionError}
                </p>
              )}
            </CardContent>
          </Card>
          <Card padding="none">
            {status.status.length === 0 ? (
              <p className="text-body-sm p-6 text-slate-500 dark:text-slate-400">No scenarios deployed yet.</p>
            ) : (
              <ul className="divide-y divide-slate-200 dark:divide-slate-700">
                {status.status.map((sc) => (
                  <li
                    key={sc.scenario_id}
                    className="flex min-h-[44px] flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="flex items-center gap-3">
                      <span className="text-body-sm font-medium text-slate-900 dark:text-white">
                        Scenario {sc.scenario_id}
                      </span>
                      {sc.in_progress && (
                        <span className="inline-flex items-center rounded-full bg-emerald-100 px-2.5 py-0.5 text-xs font-medium text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-300">
                          in progress
                        </span>
                      )}
                      {!sc.engines_reachable && (
                        <span className="inline-flex items-center rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-800 dark:bg-amber-900/30 dark:text-amber-300">
                          unreachable
                        </span>
                      )}
                    </div>
                    <div className="text-caption text-slate-500 dark:text-slate-400">
                      {sc.engines_deployed}/{sc.engines} engines deployed
                      {engineShortfall(sc) > 0 && (
                        <span className="text-amber-600 dark:text-amber-400"> · {engineShortfall(sc)} pending</span>
                      )}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </>
      )}
    </div>
  );
}
export { summarize };
export type { ReceivedMetric };
