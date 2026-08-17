import { afterEach, describe, expect, it, vi } from 'vitest';
import { getCampaignComparison } from './comparison';

describe('getCampaignComparison', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs the comparison route and passes services through unchanged', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response(
        JSON.stringify({
          campaign_id: 5,
          has_baseline: true,
          baseline_campaign_id: 4,
          services: [
            { project_id: 1, status: 'improved', has_current: true, go: true, has_baseline: true, baseline_go: false },
            { project_id: 2, status: 'still_at_risk', has_current: true, has_baseline: true },
          ],
        }),
        { status: 200, headers: { 'Content-Type': 'application/json' } }
      );
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await getCampaignComparison(5);

    expect(seenUrl).toBe('/api/campaigns/5/comparison');
    expect(got.has_baseline).toBe(true);
    expect(got.baseline_campaign_id).toBe(4);
    expect(got.services).toEqual([
      { project_id: 1, status: 'improved', has_current: true, go: true, has_baseline: true, baseline_go: false },
      { project_id: 2, status: 'still_at_risk', has_current: true, has_baseline: true },
    ]);
  });

  it('appends the explicit baseline as a query parameter', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response(JSON.stringify({ campaign_id: 5, has_baseline: true, baseline_campaign_id: 2, services: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    await getCampaignComparison(5, 2);

    expect(seenUrl).toBe('/api/campaigns/5/comparison?baseline=2');
  });

  // has_baseline:false (the tenant's first campaign, no explicit baseline)
  // is information, not an error -- callers render "no baseline", so
  // services must be a safe empty array even if a null ever reached the
  // wire.
  it('normalizes a null services array to empty', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ campaign_id: 5, has_baseline: false, services: null }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        })
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await getCampaignComparison(5);

    expect(got.has_baseline).toBe(false);
    expect(got.services).toEqual([]);
  });
});
