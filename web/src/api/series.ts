// Types and fetcher for GET /api/runs/{run_id}/series: the per-second shape
// of a run (VUs, RPS, error rate, latency percentiles), as distinct from the
// verdicts reports.ts serves. Field names mirror reportapp.SeriesPoint's
// JSON tags exactly.
import { apiClient } from './client';

export interface SeriesPoint {
  /** Unix seconds, this point's measured second; points ascend by ts. */
  ts: number;
  vus: number;
  rps: number;
  /** Failure share in percent (0-100), the series' own convention. */
  err_pct: number;
  /**
   * Percentile (as a string key, e.g. "95") to response time in seconds --
   * the domain convention; the page charts it as ms. Unset on seconds that
   * recorded no samples (Go's omitempty drops a nil map), hence optional.
   */
  latency?: Record<string, number>;
}

export interface Series {
  points: SeriesPoint[];
}

/** A run's per-second series. Runs finalised before the series store existed have a report but `points: []`. */
export function fetchSeries(runId: number): Promise<Series> {
  return apiClient.get<Series>(`/runs/${runId}/series`);
}
