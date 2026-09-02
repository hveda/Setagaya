// Types and fetchers for the capacity panel (R7): the (engine, cpu, memory)
// fan-out query and calibration job progress. Shapes mirror
// calibration_handlers.go's response structs exactly.
import { apiClient } from './client';

/** One (scenario, engine, cpu, memory) fan-out key; engine pins the executor. */
export interface CapacityKey {
  engine: string;
  cpu: string;
  memory: string;
}

/**
 * FanOutResult's status — why the engine count is or is not shown:
 * ok (profile fresh, engines valid), no_profile, stale (scenario edited
 * after calibration), target_limited (the target's health bounded the
 * search), inconclusive (search budget exhausted both-sides-healthy).
 */
export type FanOutStatus = 'ok' | 'no_profile' | 'stale' | 'target_limited' | 'inconclusive';

export interface FanOutResponse {
  status: FanOutStatus;
  /** Required engine count; meaningful ONLY when status is ok. */
  engines?: number;
}

/**
 * Turns a target aggregate QPS into an engine count. The response's status
 * is the contract: engines is present ONLY alongside "ok" — the panel
 * renders a number exclusively in that case.
 */
export function fanOutCapacity(scenarioId: number, key: CapacityKey, targetQPS: number): Promise<FanOutResponse> {
  const q = new URLSearchParams({
    engine: key.engine,
    cpu: key.cpu,
    memory: key.memory,
    target_qps: String(targetQPS),
  });
  return apiClient.get(`/scenarios/${scenarioId}/capacity-profile/fanout?${q.toString()}`);
}

export interface CapacityProfile {
  scenario_id: number;
  engine: string;
  cpu: string;
  memory: string;
  per_pod_qps: number;
  saturated_by: string;
  scenario_fingerprint: string;
  calibrated_at: string;
  job_id: number;
}

/** The stored profile for one exact key; 404 (ApiError) when none. */
export function getCapacityProfile(scenarioId: number, key: CapacityKey): Promise<CapacityProfile> {
  const q = new URLSearchParams({ engine: key.engine, cpu: key.cpu, memory: key.memory });
  return apiClient.get(`/scenarios/${scenarioId}/capacity-profile?${q.toString()}`);
}

export interface CalibrationStep {
  requested_qps: number;
  achieved_qps: number;
  classification: string;
}

export interface CalibrationJob {
  id: number;
  execution_id: number;
  /** pending -> bracketing -> bisecting -> done | failed. */
  phase: string;
  step_count: number;
  /** The QPS the job's next step will run at; absent once done/failed. */
  next_requested_qps?: number;
  result?: { saturated_by: string; per_pod_qps: number };
  failure_reason?: string;
  created_time: string;
  steps: CalibrationStep[];
}

export function getCalibrationJob(jobId: number): Promise<CalibrationJob> {
  return apiClient.get(`/calibrations/${jobId}`);
}

/**
 * Starts a fresh search over an already-configured CalibrateEngine
 * execution; the created job is returned (phase pending).
 */
export function triggerCalibration(executionId: number): Promise<CalibrationJob> {
  return apiClient.post(`/executions/${executionId}/calibration/trigger`, new URLSearchParams());
}
