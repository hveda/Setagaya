// The mounted half of the Execution page's live chart (DashboardLayout/
// Reports.test.tsx's createRoot + act style): fetch is stubbed per-URL for
// the page's snapshot endpoints, EventSource is faked for the stream, and
// the Live section's idle/disconnected/chart states are asserted as the
// DOM renders them. Execution.test.ts (the pure-function tests) stays
// untouched; these need JSX, hence a separate .tsx file.
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import Execution from './Execution';
import { SessionProvider } from '../hooks/useSession';
import type { EngineMetric, ExecutionStatus } from '../api/status';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

const metric = (over: Partial<EngineMetric> = {}): EngineMetric => ({
  threads: 2,
  latency: 0.1,
  label: 'req',
  status: '200',
  raw: '',
  execution_id: '5',
  scenario_id: '1',
  engine_id: '0',
  run_id: '1',
  ...over,
});

/** EventSource stand-in: the page's stream needs construction, handlers, close. */
class FakeEventSource {
  static instances: FakeEventSource[] = [];
  url: string;
  onopen: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }
  close() {
    this.closed = true;
  }
  open() {
    this.onopen?.();
  }
  error() {
    this.onerror?.();
  }
  emit(m: EngineMetric) {
    this.onmessage?.({ data: JSON.stringify(m) });
  }
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

const statusFixture = (phase: ExecutionStatus['phase']): ExecutionStatus => ({
  phase,
  pool_size: 0,
  status: [],
});

let container: HTMLDivElement | null = null;
let root: Root | null = null;

/** Renders /executions/5 with every fetch stubbed; info carries no engine so CapacityPanel stays away. */
async function renderExecution(phase: ExecutionStatus['phase'] = 'running') {
  container = document.createElement('div');
  document.body.appendChild(container);
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input);
      if (url.endsWith('/api/me')) {
        return json({ subject: 'demo:a', name: 'a', email: '', global_roles: [], tenants: {}, permissions: { '*': ['*'] }, demo: true });
      }
      if (url.endsWith('/api/executions/5')) {
        return json({ id: 5, name: 'demo', project_id: 1, csv_split: false, created_time: '2026-09-05T10:00:00Z', load_profile: [], data: [] });
      }
      if (url.endsWith('/api/executions/5/status')) {
        return json(statusFixture(phase));
      }
      if (url.endsWith('/api/executions/5/reports')) {
        return json([]);
      }
      return json({ message: `no stub for ${url}` }, 500);
    })
  );
  vi.stubGlobal('EventSource', FakeEventSource);
  root = createRoot(container);
  await act(async () => {
    root!.render(
      <MemoryRouter initialEntries={['/executions/5']}>
        <SessionProvider>
          <Routes>
            <Route path="/executions/:id" element={<Execution />} />
          </Routes>
        </SessionProvider>
      </MemoryRouter>
    );
  });
  // Flush the page's fetches.
  await act(async () => {});
}

afterEach(() => {
  vi.unstubAllGlobals();
  FakeEventSource.instances = [];
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

describe('Execution live section (mounted)', () => {
  it('renders the Live heading while running, disconnected first, then idle once the stream opens', async () => {
    await renderExecution('running');

    const section = container!.querySelector('[data-testid="live-section"]');
    expect(section).not.toBeNull();
    expect(section?.querySelector('h3')?.textContent).toBe('Live');

    // Before the first open: the disconnected banner, not the idle text.
    expect(container!.querySelector('[data-testid="live-disconnected"]')?.textContent).toContain(
      'Stream disconnected — reconnecting…'
    );
    expect(container!.querySelector('[data-testid="live-idle"]')).toBeNull();

    const source = FakeEventSource.instances[0];
    expect(source.url).toBe('/api/executions/5/stream');
    await act(async () => {
      source.open();
    });
    expect(container!.querySelector('[data-testid="live-disconnected"]')).toBeNull();
    expect(container!.querySelector('[data-testid="live-idle"]')?.textContent).toContain('Waiting for first events…');
  });

  it('charts streamed events and switches the latency percentile client-side', async () => {
    await renderExecution('running');
    const source = FakeEventSource.instances[0];
    await act(async () => {
      source.open();
      source.emit(metric({ threads: 3, latency: 0.12 }));
    });

    const vusChart = container!.querySelector('[data-testid="live-chart-vus-rps"]');
    expect(vusChart?.querySelector('[data-series="VUs"]')).not.toBeNull();
    expect(vusChart?.querySelector('[data-series="RPS"]')).not.toBeNull();

    // p95 is the default percentile selection.
    const latencyChart = container!.querySelector('[data-testid="live-chart-latency"]');
    expect(latencyChart?.querySelector('[data-series="p95"]')).not.toBeNull();

    const pill = container!.querySelector('[data-testid="pct-50"]') as HTMLButtonElement;
    expect(pill.getAttribute('aria-pressed')).toBe('false');
    await act(async () => {
      pill.dispatchEvent(new MouseEvent('click', { bubbles: true }));
    });
    expect(latencyChart?.querySelector('[data-series="p50"]')).not.toBeNull();
    expect(latencyChart?.querySelector('[data-series="p95"]')).toBeNull();
    expect(pill.getAttribute('aria-pressed')).toBe('true');
  });

  it('keeps the charts visible under the disconnected banner when the stream drops mid-run', async () => {
    await renderExecution('running');
    const source = FakeEventSource.instances[0];
    await act(async () => {
      source.open();
      source.emit(metric());
    });
    expect(container!.querySelector('[data-testid="live-chart-vus-rps"]')).not.toBeNull();

    await act(async () => {
      source.error();
    });
    expect(container!.querySelector('[data-testid="live-disconnected"]')).not.toBeNull();
    // The data already received stays on screen.
    expect(container!.querySelector('[data-testid="live-chart-vus-rps"]')).not.toBeNull();
    expect(container!.querySelector('[data-testid="live-idle"]')).toBeNull();
  });

  it('hides the Live section entirely when idle with nothing received', async () => {
    await renderExecution('idle');
    await act(async () => {
      FakeEventSource.instances[0]?.open();
    });
    expect(container!.querySelector('[data-testid="live-section"]')).toBeNull();
    expect(container!.textContent).not.toContain('Waiting for first events');
  });
});
