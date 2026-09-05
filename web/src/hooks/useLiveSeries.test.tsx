// The hook is behaviour, not wiring: connection tracking (onOpen/onError),
// event accumulation into series+stats, unsubscribe on unmount, and reset
// are what the Execution page consumes, so this mounts a probe with
// React's createRoot + act (useSession.test.tsx's pattern) against a fake
// EventSource -- jsdom implements none of SSE.
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useLiveSeries, type UseLiveSeriesResult } from './useLiveSeries';
import type { EngineMetric } from '../api/status';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

const metric = (over: Partial<EngineMetric> = {}): EngineMetric => ({
  threads: 2,
  latency: 0.1,
  label: 'req',
  status: '200',
  raw: '',
  execution_id: '1',
  scenario_id: '1',
  engine_id: '0',
  run_id: '1',
  ...over,
});

/** EventSource stand-in: the hook only needs construction, handler assignment, and close(). */
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
  /** The server accepted the stream. */
  open() {
    this.onopen?.();
  }
  /** The connection dropped; a real EventSource would auto-retry. */
  error() {
    this.onerror?.();
  }
  emit(m: EngineMetric) {
    this.onmessage?.({ data: JSON.stringify(m) });
  }
}

let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderProbe(executionId: number, enabled: boolean): Promise<{ current: UseLiveSeriesResult | null }> {
  container = document.createElement('div');
  document.body.appendChild(container);
  const captured: { current: UseLiveSeriesResult | null } = { current: null };
  function Probe() {
    captured.current = useLiveSeries(executionId, enabled);
    return null;
  }
  vi.stubGlobal('EventSource', FakeEventSource);
  root = createRoot(container);
  await act(async () => {
    root!.render(<Probe />);
  });
  return captured;
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

describe('useLiveSeries', () => {
  it('subscribes to the execution stream when enabled and stays silent when not', async () => {
    const captured = await renderProbe(7, true);
    expect(FakeEventSource.instances).toHaveLength(1);
    expect(FakeEventSource.instances[0].url).toBe('/api/executions/7/stream');
    expect(captured.current).toMatchObject({ series: [], connected: false, lastEventAt: null });
  });

  it('does not construct an EventSource while disabled', async () => {
    await renderProbe(7, false);
    expect(FakeEventSource.instances).toHaveLength(0);
  });

  it('tracks connection through open/error (a real source refires open after retrying)', async () => {
    const captured = await renderProbe(1, true);
    const source = FakeEventSource.instances[0];
    expect(captured.current?.connected).toBe(false);

    await act(async () => {
      source.open();
    });
    expect(captured.current?.connected).toBe(true);

    await act(async () => {
      source.error();
    });
    expect(captured.current?.connected).toBe(false);

    await act(async () => {
      source.open();
    });
    expect(captured.current?.connected).toBe(true);
  });

  it('accumulates events into the chart series and the rolling stats', async () => {
    const captured = await renderProbe(1, true);
    const source = FakeEventSource.instances[0];

    await act(async () => {
      source.emit(metric({ threads: 2, latency: 0.1 }));
      source.emit(metric({ threads: 5, latency: 0.2 }));
    });

    // Both events landed in the same second: one bucket.
    expect(captured.current?.series).toHaveLength(1);
    expect(captured.current?.series[0]).toMatchObject({ t: 0, vus: 5, rps: 2 });
    expect(captured.current?.lastEventAt).not.toBeNull();
    // summarize()'s trailing-10s numbers: 2 events / 10s.
    expect(captured.current?.stats.throughput).toBeCloseTo(0.2, 10);
    expect(captured.current?.stats.latencySeconds).toBe(0.2);
  });

  it('closes the EventSource on unmount', async () => {
    await renderProbe(1, true);
    const source = FakeEventSource.instances[0];
    expect(source.closed).toBe(false);
    await act(async () => {
      root!.unmount();
    });
    expect(source.closed).toBe(true);
  });

  it('reset() drops everything received without tearing down the stream', async () => {
    const captured = await renderProbe(1, true);
    const source = FakeEventSource.instances[0];
    await act(async () => {
      source.open();
      source.emit(metric());
    });
    expect(captured.current?.series).toHaveLength(1);

    await act(async () => {
      captured.current?.reset();
    });
    expect(captured.current?.series).toEqual([]);
    expect(captured.current?.stats).toEqual({ throughput: 0, errorRate: 0, latencySeconds: null });
    expect(captured.current?.lastEventAt).toBeNull();
    expect(captured.current?.connected).toBe(true);
    expect(source.closed).toBe(false);
  });
});
