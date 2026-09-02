// Types and fetchers for scenario fragments (R4's editor data plane):
// GET /api/scenarios/{id}/requests returns the fragment's YAML bytes
// verbatim (text/yaml, G2); PUT takes them back the same way (G3, any
// of the text/yaml media types). Round-tripping is byte-exact by
// contract -- the editor must never reformat what it did not touch.
import { apiClient } from './client';

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
