import { afterEach, describe, expect, it, vi } from 'vitest';
import { listExecutionReports } from './reports';

describe('listExecutionReports', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  // The backend encodes ListReports' nil slice as JSON null, not [], for an
  // execution with no reports yet (confirmed against a real cmd/api: `GET
  // /api/executions/1/reports` on a fresh fake store returns "null"). A
  // caller treating the result as an array (e.g. `.length`) would otherwise
  // throw at runtime.
  it('normalizes a null response body to an empty array', async () => {
    const fetchMock = vi.fn(async () => new Response('null', { status: 200, headers: { 'Content-Type': 'application/json' } }));
    vi.stubGlobal('fetch', fetchMock);

    const got = await listExecutionReports(1);

    expect(got).toEqual([]);
  });

  it('passes through a real report array unchanged', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify([{ run_id: 1, outcome: 'passed' }]), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await listExecutionReports(1);

    expect(got).toHaveLength(1);
    expect(got[0].run_id).toBe(1);
  });

  it('appends limit as a query parameter when given', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    await listExecutionReports(1, 10);

    expect(seenUrl).toBe('/api/executions/1/reports?limit=10');
  });
});
