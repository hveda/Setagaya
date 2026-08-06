import { afterEach, describe, expect, it, vi } from 'vitest';
import { getExecutionStatus } from './status';

describe('getExecutionStatus', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // The backend encodes lifecycleapp.Status's nil []ScenarioStatus as JSON
  // null, not [], when no scenarios are deployed yet (confirmed against a
  // real cmd/api: GET /api/executions/999/status on a fresh fake store
  // returns {"phase":"idle","pool_size":0,"status":null}). A caller treating
  // the result as an array (e.g. `.length`) would otherwise throw at runtime.
  it('normalizes a null status array to an empty array', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ phase: 'idle', pool_size: 0, status: null }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await getExecutionStatus(999);

    expect(got.status).toEqual([]);
  });

  it('passes through a real status array unchanged', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            phase: 'running',
            pool_size: 2,
            status: [{ scenario_id: 1, engines: 2, engines_deployed: 2, engines_reachable: true, in_progress: true }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } }
        )
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await getExecutionStatus(1);

    expect(got.status).toHaveLength(1);
    expect(got.status[0].scenario_id).toBe(1);
  });
});
