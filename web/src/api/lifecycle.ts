// Types and fetchers for the execution hub's lifecycle actions (R2):
// deploy / trigger / stop / purge on /api/executions/{id}/{action}. The
// backend's mutating routes are form-encoded (see ApiClient.post), and a
// 409 surfaces the server's message verbatim via ApiError -- the hub
// renders that, not a generic failure.
import { apiClient } from './client';

/** The server's {"message": "..."} shape on a successful mutation. */
interface MutationOk {
  message: string;
}

function lifecycleAction(executionId: number, action: 'deploy' | 'trigger' | 'stop' | 'purge'): Promise<string> {
  return apiClient.post<MutationOk>(`/executions/${executionId}/${action}`, new URLSearchParams()).then(
    (r) => r.message,
  );
}

export const deployExecution = (id: number) => lifecycleAction(id, 'deploy');
export const triggerExecution = (id: number) => lifecycleAction(id, 'trigger');
export const stopExecution = (id: number) => lifecycleAction(id, 'stop');
export const purgeExecution = (id: number) => lifecycleAction(id, 'purge');
