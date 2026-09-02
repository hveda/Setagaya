import { describe, expect, it, vi } from 'vitest';
import { deployExecution, purgeExecution, stopExecution, triggerExecution } from './lifecycle';
import { apiClient } from './client';

describe('lifecycle actions', () => {
  it('POSTs form-encoded to the right action routes', async () => {
    const spy = vi.spyOn(apiClient, 'post').mockResolvedValue({ message: 'ok' });
    await deployExecution(7);
    await triggerExecution(7);
    await stopExecution(7);
    await purgeExecution(7);
    const calls = spy.mock.calls.map(([path]) => path);
    expect(calls).toEqual([
      '/executions/7/deploy',
      '/executions/7/trigger',
      '/executions/7/stop',
      '/executions/7/purge',
    ]);
    // Every call is form-encoded (the backend's only mutating shape).
    for (const [, body] of spy.mock.calls) {
      expect(body).toBeInstanceOf(URLSearchParams);
    }
    spy.mockRestore();
  });

  it('propagates ApiError verbatim (409 message survives)', async () => {
    // The hub renders the server's message; this pins that the api layer
    // does not swallow or rewrite it.
    vi.spyOn(apiClient, 'post').mockRejectedValue(new Error('engines not ready'));
    await expect(triggerExecution(7)).rejects.toThrow('engines not ready');
    vi.restoreAllMocks();
  });
});
