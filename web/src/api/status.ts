// Types and fetchers for the live status page's two data sources: GET
// /api/executions/{execution_id}/status (a snapshot) and GET
// /api/executions/{execution_id}/stream (an SSE feed of engine.Metric
// events, one per measured interval). Field names mirror the Go JSON tags
// exactly -- see internal/app/lifecycleapp/service.go's Status/ScenarioStatus
// and internal/domain/engine/engine.go's Metric.
import { apiClient } from './client';

export type Phase = 'idle' | 'deployed' | 'running';

export interface ScenarioStatus {
  scenario_id: number;
  engines: number;
  engines_deployed: number;
  engines_reachable: boolean;
  in_progress: boolean;
  started_time?: string;
}

export interface ExecutionStatus {
  phase: Phase;
  pool_size: number;
  status: ScenarioStatus[];
}

export async function getExecutionStatus(executionId: number): Promise<ExecutionStatus> {
  const got = await apiClient.get<ExecutionStatus>(`/executions/${executionId}/status`);
  // Go marshals a nil []ScenarioStatus as JSON null, not [], when no
  // scenarios are deployed yet (see internal/app/lifecycleapp's Status).
  return { ...got, status: got.status ?? [] };
}

export interface EngineMetric {
  threads: number;
  latency: number;
  label: string;
  status: string;
  raw: string;
  execution_id: string;
  scenario_id: string;
  engine_id: string;
  run_id: string;
}

/** Subscribes to an execution's live metric stream. Returns an unsubscribe function. */
export function streamExecutionMetrics(executionId: number, onMetric: (m: EngineMetric) => void): () => void {
  const source = new EventSource(`${apiClient.baseUrl}/executions/${executionId}/stream`);
  source.onmessage = (event: MessageEvent<string>) => {
    try {
      onMetric(JSON.parse(event.data) as EngineMetric);
    } catch {
      // Malformed event; drop it rather than take down the whole stream.
    }
  };
  return () => source.close();
}
