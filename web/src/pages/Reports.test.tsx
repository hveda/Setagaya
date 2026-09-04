import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { parseShard, default as Reports } from './Reports';
import type { Report } from '../api/reports';
import type { SeriesPoint } from '../api/series';

describe('parseShard', () => {
  it('parses 0-indexed shard numbers', () => {
    expect(parseShard('0')).toBe(0);
    expect(parseShard('3')).toBe(3);
    expect(parseShard(' 2 ')).toBe(2);
  });

  // Number('') is 0 in JS -- the empty-string guard keeps an empty input
  // from silently loading shard 0.
  it('rejects an empty input rather than treating it as shard 0', () => {
    expect(parseShard('')).toBeNull();
    expect(parseShard('  ')).toBeNull();
  });

  it('rejects negatives, fractions, and junk', () => {
    expect(parseShard('-1')).toBeNull();
    expect(parseShard('1.5')).toBeNull();
    expect(parseShard('abc')).toBeNull();
  });
});

// The mounted half, DashboardLayout.test.tsx's style: createRoot + act,
// fetch stubbed per-URL so the report and series endpoints the detail page
// fires side by side are both under test.
(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

const reportFixture: Report = {
  execution_id: 1,
  scenario_id: 2,
  run_id: 9,
  started_at: '2026-09-04T10:00:00Z',
  ended_at: '2026-09-04T10:01:00Z',
  outcome: 'passed',
  requested: { concurrency: 10, throughput: 100 },
  achieved: { concurrency: 10, throughput: 95, samples: 6000, failed: 3 },
  error_rate: 0.0005,
  latency: { '50': 0.05, '95': 0.2 },
  attribution: { target: 2, engine: 1, unknown: 0 },
};

const seriesFixture: SeriesPoint[] = [
  { ts: 1_700_000_000, vus: 10, rps: 100, err_pct: 0.5, latency: { '50': 0.05, '90': 0.1, '95': 0.2, '99': 0.4 } },
  { ts: 1_700_000_001, vus: 10, rps: 102, err_pct: 0, latency: { '50': 0.055, '90': 0.11, '95': 0.21, '99': 0.41 } },
  { ts: 1_700_000_002, vus: 8, rps: 80, err_pct: 1.5, latency: { '50': 0.06, '90': 0.12, '95': 0.22, '99': 0.42 } },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderReportDetail(
  seriesBody: () => Response = () => json({ points: seriesFixture }),
  calls: string[] = []
) {
  container = document.createElement('div');
  document.body.appendChild(container);
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      calls.push(url);
      if (url.endsWith('/api/runs/9/report')) {
        return json(reportFixture);
      }
      if (url.endsWith('/api/runs/9/series')) {
        return seriesBody();
      }
      return json({ message: `no stub for ${url}` }, 500);
    })
  );
  root = createRoot(container);
  await act(async () => {
    root!.render(
      <MemoryRouter initialEntries={['/reports/9']}>
        <Routes>
          <Route path="/reports/:runId" element={<Reports />} />
        </Routes>
      </MemoryRouter>
    );
  });
  // Flush the report + series fetches' promise chains.
  await act(async () => {});
}

afterEach(() => {
  vi.unstubAllGlobals();
  const r = root;
  if (r !== null && container !== null) {
    act(() => {
      r.unmount();
    });
  }
  container?.remove();
  container = null;
  root = null;
});

describe('ReportDetail time series (mounted)', () => {
  it('renders three charts from the run\'s series points', async () => {
    await renderReportDetail();

    expect(container!.textContent).toContain('Time series');
    expect(container!.querySelectorAll('svg[role="img"]').length).toBe(3);
    expect(container!.querySelector('[data-series="VUs"]')).not.toBeNull();
    expect(container!.querySelector('[data-series="RPS"]')).not.toBeNull();
    expect(container!.querySelector('[data-series="error %"]')).not.toBeNull();
    // p95 is the default percentile selection.
    expect(container!.querySelector('[data-series="p95"]')).not.toBeNull();
    // The time axis formats ticks as wall-clock times.
    const xLabels = Array.from(container!.querySelectorAll('svg text')).map((t) => t.textContent);
    expect(xLabels.some((l) => /\d{2}:\d{2}:\d{2}/.test(l ?? ''))).toBe(true);
  });

  it('switches the latency percentile client-side: no refetch, new series', async () => {
    const calls: string[] = [];
    await renderReportDetail(() => json({ points: seriesFixture }), calls);
    const seriesCallsBefore = calls.filter((u) => u.endsWith('/api/runs/9/series')).length;
    expect(seriesCallsBefore).toBe(1);

    const pill = container!.querySelector('[data-testid="pct-50"]') as HTMLButtonElement;
    expect(pill).not.toBeNull();
    await act(async () => {
      pill.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });

    expect(calls.filter((u) => u.endsWith('/api/runs/9/series')).length).toBe(seriesCallsBefore);
    expect(container!.querySelector('[data-series="p50"]')).not.toBeNull();
    expect(container!.querySelector('[data-series="p95"]')).toBeNull();
    // The pill reflects the selection.
    expect(pill.getAttribute('aria-pressed')).toBe('true');
  });

  it('charts nothing but says so when the run has no series (pre-series-store runs)', async () => {
    await renderReportDetail(() => json({ points: [] }));

    expect(container!.querySelector('[data-testid="series-empty"]')).not.toBeNull();
    expect(container!.querySelectorAll('svg[role="img"]').length).toBe(0);
  });

  it('shows the error with a retry that refetches', async () => {
    const calls: string[] = [];
    await renderReportDetail(() => json({ message: 'series backend down' }, 500), calls);

    const alert = container!.querySelector('[role="alert"]');
    expect(alert?.textContent).toContain('series backend down');
    expect(container!.querySelector('[data-testid="series-retry"]')).not.toBeNull();

    await act(async () => {
      container!
        .querySelector('[data-testid="series-retry"]')!
        .dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });
    expect(calls.filter((u) => u.endsWith('/api/runs/9/series')).length).toBe(2);
  });
});
