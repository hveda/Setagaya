import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './client';
import { fetchSeries } from './series';

// reports.test.ts conventions: stub global fetch (the transport apiClient
// wraps) rather than the client itself, so URL construction and the error
// envelope mapping are both under test.
describe('fetchSeries', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs /runs/{run_id}/series and parses the points envelope', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response(
        JSON.stringify({
          points: [
            { ts: 1_700_000_000, vus: 10, rps: 100.5, err_pct: 1.25, latency: { '50': 0.05, '95': 0.2 } },
            { ts: 1_700_000_001, vus: 12, rps: 118, err_pct: 0, latency: { '50': 0.06, '95': 0.22 } },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      );
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchSeries(9);

    expect(seenUrl).toBe('/api/runs/9/series');
    expect(got.points).toHaveLength(2);
    expect(got.points[0]).toEqual({
      ts: 1_700_000_000,
      vus: 10,
      rps: 100.5,
      err_pct: 1.25,
      latency: { '50': 0.05, '95': 0.2 },
    });
  });

  it('parses points with latency omitted (seconds that recorded no samples)', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ points: [{ ts: 1_700_000_010, vus: 5, rps: 40, err_pct: 0 }] }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await fetchSeries(9);

    expect(got.points[0].latency).toBeUndefined();
  });

  it('maps the backend error envelope to a typed ApiError', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ message: 'series not configured' }), {
          status: 404,
          headers: { 'Content-Type': 'application/json' },
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const err = await fetchSeries(9).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(404);
    expect((err as ApiError).message).toBe('series not configured');
  });
});
