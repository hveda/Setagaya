// Types and fetchers for scenario fragments (R4's editor data plane):
// GET /api/scenarios/{id}/requests returns the fragment's YAML bytes
// verbatim (text/yaml, G2); PUT takes them back the same way (G3, any
// of the text/yaml media types). Round-tripping is byte-exact by
// contract -- the editor must never reformat what it did not touch.
import { apiClient, ApiError } from './client';

/** The fragment's YAML exactly as stored (no server-side normalization). */
export function getScenarioRequests(scenarioId: number): Promise<string> {
  return apiClient.text(`/scenarios/${scenarioId}/requests`);
}

/**
 * Saves the fragment. The body is sent verbatim as text/yaml -- the G3
 * handler dispatches on media type and stores non-multipart bodies
 * byte-for-byte.
 */
export function setScenarioRequests(scenarioId: number, yaml: string): Promise<void> {
  return apiClient.putRaw(`/scenarios/${scenarioId}/requests`, 'text/yaml; charset=utf-8', yaml);
}

/** One finding from G4/G6, line-anchored (mirrors scenarioapp.Diagnostic). */
export interface Diagnostic {
  severity: 'error' | 'info';
  message: string;
  line: number;
  col?: number;
  path?: string;
}

export interface ValidateResponse {
  valid: boolean;
  diagnostics: Diagnostic[];
}

/**
 * G5's validate endpoint: same checks as store, nothing persisted. A 400
 * carries DiagnosticsError -- the reasons ride in the error body's
 * diagnostics array, unwrapped here. Other errors propagate.
 */
export async function validateScenarioRequests(
  scenarioId: number,
  yaml: string,
): Promise<ValidateResponse> {
  try {
    await apiClient.putRaw(`/scenarios/${scenarioId}/requests/validate`, 'text/yaml; charset=utf-8', yaml);
    return { valid: true, diagnostics: [] };
  } catch (err) {
    if (err instanceof ApiError && err.status === 400) {
      const diags = (err.data as { diagnostics?: Diagnostic[] } | undefined)?.diagnostics;
      return { valid: false, diagnostics: diags ?? [] };
    }
    throw err;
  }
}
