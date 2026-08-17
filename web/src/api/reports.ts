// Types and fetchers for the two read-only report endpoints this page uses:
// GET /api/executions/{execution_id}/reports and GET /api/runs/{run_id}/report.
// Field names mirror internal/domain/report.Report's JSON tags exactly.
import { apiClient } from './client';

export interface Load {
  concurrency: number;
  throughput: number;
  duration_seconds?: number;
  samples?: number;
  failed?: number;
}

export interface Attribution {
  target: number;
  engine: number;
  unknown: number;
}

export type ErrorSide = 'target' | 'engine' | 'unknown';

export interface ErrorSignature {
  label: string;
  response_code?: string;
  side: ErrorSide;
  count: number;
  exemplars?: string[];
}

export interface LabelSummary {
  label: string;
  samples: number;
  failed: number;
  error_rate: number;
  latency: Record<string, number>;
}

export type Outcome = 'passed' | 'failed' | 'aborted' | 'error';

export interface Report {
  execution_id: number;
  scenario_id: number;
  run_id: number;
  engine?: string;
  /** Load origin: empty/absent means the deployment default cluster. */
  cluster?: string;
  /** Trace id the run's load carried (traceparent/baggage); absent on runs that predate it. */
  correlation_id?: string;
  started_at: string;
  ended_at: string;
  outcome: Outcome;
  requested: Load;
  achieved: Load;
  error_rate: number;
  /** Percentile (as a string, e.g. "95") -> response time in seconds. */
  latency: Record<string, number>;
  attribution: Attribution;
  errors?: ErrorSignature[];
  labels?: LabelSummary[];
}

/**
 * Most recent first, per the backend's own ordering (ListReports). The
 * backend encodes a nil slice as JSON null rather than [] (Go's json
 * package does this for an unset slice) -- normalized here so callers can
 * always treat the result as an array.
 */
export async function listExecutionReports(executionId: number, limit?: number): Promise<Report[]> {
  const query = limit ? `?limit=${limit}` : '';
  const got = await apiClient.get<Report[] | null>(`/executions/${executionId}/reports${query}`);
  return got ?? [];
}

export function getRunReport(runId: number): Promise<Report> {
  return apiClient.get<Report>(`/runs/${runId}/report`);
}
