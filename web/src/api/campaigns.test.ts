import { afterEach, describe, expect, it, vi } from 'vitest';
import { createCampaign, getCampaign, getCampaignVerdict, listTenantCampaigns } from './campaigns';

describe('campaigns api', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('createCampaign posts a form-encoded body to the tenant-scoped route', async () => {
    let seenUrl = '';
    let seenMethod = '';
    let seenContentType: string | null = null;
    let seenBody = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      seenUrl = String(input);
      seenMethod = init?.method ?? '';
      seenContentType = new Headers(init?.headers).get('Content-Type');
      seenBody = String(init?.body);
      return new Response(JSON.stringify({ id: 1, name: 'c', tenant_id: 7, window_start: 'a', window_end: 'b', services: [], active: false }), {
        status: 201,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const windowStart = new Date('2026-08-07T00:00:00Z');
    const windowEnd = new Date('2026-08-07T01:00:00Z');
    const got = await createCampaign(7, {
      name: 'Launch Readiness',
      windowStart,
      windowEnd,
      services: [
        { project_id: 1, execution_id: 10 },
        { project_id: 2, execution_id: 20 },
      ],
    });

    expect(seenUrl).toBe('/api/tenants/7/campaigns');
    expect(seenMethod).toBe('POST');
    expect(seenContentType).toBe('application/x-www-form-urlencoded');
    const body = new URLSearchParams(seenBody);
    expect(body.get('name')).toBe('Launch Readiness');
    expect(body.get('window_start')).toBe(windowStart.toISOString());
    expect(body.get('window_end')).toBe(windowEnd.toISOString());
    expect(body.getAll('service_project_id')).toEqual(['1', '2']);
    expect(body.getAll('service_execution_id')).toEqual(['10', '20']);
    expect(got.id).toBe(1);
  });

  it('listTenantCampaigns GETs the tenant-scoped route', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response('[]', { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    await listTenantCampaigns(7);

    expect(seenUrl).toBe('/api/tenants/7/campaigns');
  });

  it('getCampaign GETs a single campaign by id', async () => {
    let seenUrl = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      seenUrl = String(input);
      return new Response(JSON.stringify({ id: 5, name: 'c', tenant_id: 7, window_start: 'a', window_end: 'b', services: [], active: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await getCampaign(5);

    expect(seenUrl).toBe('/api/campaigns/5');
    expect(got.id).toBe(5);
  });

  describe('getCampaignVerdict', () => {
    // failing_criteria and other_load are `omitempty` on the wire: absent
    // entirely (not []) when there's nothing to report -- confirmed against
    // internal/adapters/httpapi/campaign_handlers_test.go's
    // TestGetCampaignVerdict_ReturnsPerServiceOutcomeAndOverallGo, which
    // decodes failing_criteria as nil for a passed service.
    it('normalizes absent failing_criteria and other_load to empty arrays', async () => {
      const fetchMock = vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              campaign_id: 1,
              go: true,
              services: [{ project_id: 1, execution_id: 10, has_report: true, outcome: 'passed' }],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } }
          )
      );
      vi.stubGlobal('fetch', fetchMock);

      const got = await getCampaignVerdict(1);

      expect(got.services[0].failing_criteria).toEqual([]);
      expect(got.other_load).toEqual([]);
    });

    it('passes through populated failing_criteria and other_load unchanged', async () => {
      const fetchMock = vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              campaign_id: 1,
              go: false,
              services: [
                {
                  project_id: 1,
                  execution_id: 10,
                  has_report: true,
                  outcome: 'failed',
                  failing_criteria: [{ criterion: 'failures>10%' }],
                },
              ],
              other_load: [{ execution_id: 99, start: 'a', end: 'b', engine_count: 3 }],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } }
          )
      );
      vi.stubGlobal('fetch', fetchMock);

      const got = await getCampaignVerdict(1);

      expect(got.services[0].failing_criteria).toEqual([{ criterion: 'failures>10%' }]);
      expect(got.other_load).toEqual([{ execution_id: 99, start: 'a', end: 'b', engine_count: 3 }]);
    });
  });
});
