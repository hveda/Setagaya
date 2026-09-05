// The Execution page's live stream, extracted from its inline predecessor
// (eventsRef + prune + 1s ticker + subscribe in Execution.tsx). The hook
// owns the EventSource lifecycle and the raw-event window; the aggregation
// itself lives in lib/liveSeries.ts so it stays pure and unit-testable.
import { useCallback, useEffect, useRef, useState } from 'react';
import { streamExecutionMetrics } from '../api/status';
import { liveSeries, summarize, windowMs, type LiveSeriesPoint, type LiveStats, type ReceivedMetric } from '../lib/liveSeries';

/**
 * Raw events outlive summarize()'s 10s stat window: the chart keeps 60
 * per-second buckets, so retention covers that span plus the forming edge
 * of the newest bucket. The reducer caps the bucket count anyway.
 */
const retentionMs = 61_000;

const zeroStats: LiveStats = { throughput: 0, errorRate: 0, latencySeconds: null };

/** What useLiveSeries exposes: the chart's series, the stream's health, and the page's rolling numbers. */
export interface UseLiveSeriesResult {
  series: LiveSeriesPoint[];
  /** True while the EventSource is open; false while connecting or auto-reconnecting after a drop. */
  connected: boolean;
  /** Receipt time (ms epoch) of the most recent event, null before the first. */
  lastEventAt: number | null;
  /** The three trailing-10s rolling numbers, same meaning as before the extraction. */
  stats: LiveStats;
  /** Drops everything received so far (used after purge, as the inline code did for the stat cards). */
  reset: () => void;
}

/**
 * Subscribes to an execution's metric stream while enabled and keeps two
 * derived views of the same raw events: the per-second chart series (60s
 * rolling) and the trailing-10s rolling numbers. Numbers are recomputed
 * on every event and once per second, exactly as the inline code did.
 */
export function useLiveSeries(executionId: number, enabled: boolean): UseLiveSeriesResult {
  const [series, setSeries] = useState<LiveSeriesPoint[]>([]);
  const [connected, setConnected] = useState(false);
  const [lastEventAt, setLastEventAt] = useState<number | null>(null);
  const [stats, setStats] = useState<LiveStats>(zeroStats);
  const eventsRef = useRef<ReceivedMetric[]>([]);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    eventsRef.current = [];
    setSeries([]);
    setConnected(false);
    setLastEventAt(null);
    setStats(zeroStats);
    const recompute = () => {
      const now = Date.now();
      eventsRef.current = eventsRef.current.filter((e) => now - e.receivedAt < retentionMs);
      setSeries(liveSeries(eventsRef.current));
      setStats(summarize(eventsRef.current.filter((e) => now - e.receivedAt < windowMs)));
    };
    const unsubscribe = streamExecutionMetrics(
      executionId,
      (metric) => {
        const receivedAt = Date.now();
        eventsRef.current = [...eventsRef.current, { receivedAt, metric }];
        setLastEventAt(receivedAt);
        recompute();
      },
      {
        onOpen: () => setConnected(true),
        onError: () => setConnected(false),
      }
    );
    const ticker = setInterval(recompute, 1000);
    return () => {
      unsubscribe();
      clearInterval(ticker);
    };
  }, [executionId, enabled]);

  const reset = useCallback(() => {
    eventsRef.current = [];
    setSeries([]);
    setStats(zeroStats);
    setLastEventAt(null);
  }, []);

  return { series, connected, lastEventAt, stats, reset };
}
