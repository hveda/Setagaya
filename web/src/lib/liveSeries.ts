// Pure aggregation of an execution's live metric stream into per-second
// buckets for the Execution page's live chart. No React, no SSE imports --
// hooks/useLiveSeries.ts owns the stream; this module only turns a window
// of received events into chart points, so it stays unit-testable.
import type { EngineMetric } from '../api/status';

/** One stream event as received client-side: the metric plus its local receipt time (ms epoch). */
export interface ReceivedMetric {
  receivedAt: number;
  metric: EngineMetric;
}

/**
 * One per-second bucket of the live chart. t is seconds since the earliest
 * event in the window (NOT wall-clock: the live view charts run-relative
 * time), p50/p95/p99 are response-time percentiles in seconds -- the wire's
 * domain convention, multiplied to ms at chart time like Reports does.
 */
export interface LiveSeriesPoint {
  t: number;
  vus: number;
  rps: number;
  errPct: number;
  p50: number;
  p95: number;
  p99: number;
}

/** How many per-second buckets the live chart keeps (the rolling window). */
export const maxBuckets = 60;

/**
 * The trailing window behind the Execution page's three rolling numbers,
 * unchanged from its inline predecessor in Execution.tsx.
 */
export const windowMs = 10_000;

/** The three rolling numbers the Execution page's stat cards show. */
export interface LiveStats {
  throughput: number;
  errorRate: number;
  latencySeconds: number | null;
}

/**
 * Trailing-window summary of received events, moved verbatim from
 * Execution.tsx when the stream logic was extracted into
 * hooks/useLiveSeries.ts: throughput = events per second over the window,
 * errorRate = failed share (status !== '200'), latency = the most recent
 * event's latency. The stream carries one event per measured interval
 * (roughly once a second per active label/shard), not one per request --
 * there is no per-request count to work with here, only what the interval
 * already summarised. A trailing window over received events is the
 * closest "current" signal obtainable without adding a new backend
 * aggregation.
 */
export function summarize(events: ReceivedMetric[]): LiveStats {
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
 * An event counts as failed when its status is not "200". The stream's
 * status field carries HTTP-code strings -- the ingest path
 * (internal/app/metricsapp/ingest.go record()) emits exactly "200" or
 * "500" -- so this is summarize()'s existing convention, not a
 * "success"/"ok" check (neither ever appears on the wire).
 */
function isFailed(m: EngineMetric): boolean {
  return m.status !== '200';
}

/**
 * Nearest-rank percentile: the ceil(p/100 * N)-th smallest value
 * (1-indexed), clamped into range. Chosen over linear interpolation
 * because per-second buckets are small (often a single sample) and
 * nearest-rank never reports a latency no event actually measured.
 */
function nearestRank(sortedAsc: number[], p: number): number {
  const rank = Math.min(Math.max(1, Math.ceil((p / 100) * sortedAsc.length)), sortedAsc.length);
  return sortedAsc[rank - 1];
}

/**
 * Buckets received events by floor((receivedAt - first receipt)/1000) and
 * returns at most the last maxBuckets buckets, ascending by t:
 * vus = max threads in the bucket, rps = event count (each event is one
 * measured interval per label/shard -- the closest request-rate proxy the
 * stream offers, the same convention as the trailing samples/sec stat),
 * errPct = failed share * 100, percentiles over the bucket's latencies.
 * Labels are aggregated into the same bucket: the live chart shows the
 * run's total shape. Seconds with no events are simply absent (gaps stay
 * visible), and out-of-order arrivals are anchored on the EARLIEST
 * receivedAt so every bucket index is >= 0.
 */
export function liveSeries(events: ReceivedMetric[]): LiveSeriesPoint[] {
  if (events.length === 0) {
    return [];
  }
  const firstTs = Math.min(...events.map((e) => e.receivedAt));
  const buckets = new Map<number, ReceivedMetric[]>();
  for (const e of events) {
    const second = Math.floor((e.receivedAt - firstTs) / 1000);
    const list = buckets.get(second);
    if (list) {
      list.push(e);
    } else {
      buckets.set(second, [e]);
    }
  }
  return Array.from(buckets.entries())
    .sort(([a], [b]) => a - b)
    .slice(-maxBuckets)
    .map(([second, list]) => {
      const latencies = list.map((e) => e.metric.latency).sort((a, b) => a - b);
      const failed = list.filter((e) => isFailed(e.metric)).length;
      return {
        t: second,
        vus: Math.max(...list.map((e) => e.metric.threads)),
        rps: list.length,
        errPct: (failed / list.length) * 100,
        p50: nearestRank(latencies, 50),
        p95: nearestRank(latencies, 95),
        p99: nearestRank(latencies, 99),
      };
    });
}
