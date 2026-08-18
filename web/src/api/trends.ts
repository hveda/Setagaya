// Types and fetchers for the report analytics endpoints: GET
// /api/executions/{execution_id}/trend and GET
// /api/executions/{execution_id}/error-signatures. Field names mirror
// internal/adapters/httpapi/report_handlers.go's trendResponse and
// errorSignatureHistoryResponse JSON tags exactly.
import { apiClient } from './client';

export interface TrendPoint {
  run_id: number;
  outcome: string;
  achieved_throughput: number;
  requested_throughput: number;
  error_rate: number;
  p50: number;
  p90: number;
  p95: number;
  p99: number;
  hit_target_qps: boolean;
  /** False = "no baseline": no comparable predecessor, never a regression. */
  has_comparable_predecessor: boolean;
  regressed?: boolean;
}

export interface Trend {
  execution_id: number;
  /** Most recent first, per ListReports' ordering. */
  points: TrendPoint[];
}

/**
 * `points` needs no null normalization: toTrendResponse always builds it
 * with make(), so the wire carries [] (never null) even for an execution
 * with no runs.
 */
export function getExecutionTrend(executionId: number, limit?: number): Promise<Trend> {
  const query = limit ? `?limit=${limit}` : '';
  return apiClient.get<Trend>(`/executions/${executionId}/trend${query}`);
}

/** Grouping axis for the error-signature history. */
export type SignatureGroupBy = 'label' | 'code';

export type SignatureSide = 'target' | 'engine' | 'unknown';

export interface SignatureHistoryRow {
  label: string;
  response_code?: string;
  side: SignatureSide;
  total_count: number;
  /** Meaningful per leaf row only -- never rolled up to the group. */
  run_count: number;
}

export interface SignatureBreakdown {
  key: string;
  /** Safe re-sum of the group's rows' total_counts. */
  total_count: number;
  rows: SignatureHistoryRow[];
}

export interface ErrorSignatureHistory {
  execution_id: number;
  /** Echoes the grouping: "label" or "response_code" (the Go constant, not the ?by= value). */
  grouped_by: string;
  groups: SignatureBreakdown[];
}

/**
 * `by` defaults to label server-side; only the non-default axis needs
 * passing. `groups`/`rows` need no null normalization:
 * toErrorSignatureHistoryResponse always builds them with make().
 */
export function getErrorSignatures(executionId: number, by?: SignatureGroupBy): Promise<ErrorSignatureHistory> {
  const query = by === 'code' ? '?by=code' : '';
  return apiClient.get<ErrorSignatureHistory>(`/executions/${executionId}/error-signatures${query}`);
}
