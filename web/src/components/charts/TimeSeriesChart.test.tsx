import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import TimeSeriesChart, { axisTicks, nearestPoint } from './TimeSeriesChart';
import type { TimeSeriesPoint } from './TimeSeriesChart';

// Sparkline's helper-test convention, plus the mounted-DOM half in
// DashboardLayout.test.tsx's style: createRoot + act, no testing-library.
describe('nearestPoint', () => {
  const points: TimeSeriesPoint[] = [
    { x: 0, y: 10 },
    { x: 1, y: 20 },
    { x: 2, y: 30 },
    { x: 3, y: 40 },
  ];

  it('returns null for an empty series', () => {
    expect(nearestPoint([], 5)).toBeNull();
  });

  it('returns the exact sample when x matches one', () => {
    expect(nearestPoint(points, 2)).toEqual({ x: 2, y: 30 });
  });

  it('picks the nearer sample between two', () => {
    expect(nearestPoint(points, 1.6)).toEqual({ x: 2, y: 30 });
    expect(nearestPoint(points, 1.4)).toEqual({ x: 1, y: 20 });
  });

  // Ties must be deterministic, not coin-flip order of a sort.
  it('breaks ties toward the earlier sample', () => {
    expect(nearestPoint(points, 0.5)).toEqual({ x: 0, y: 10 });
    expect(nearestPoint(points, 2.5)).toEqual({ x: 2, y: 30 });
  });
});

describe('axisTicks', () => {
  it('returns nothing for an inverted or non-finite domain', () => {
    expect(axisTicks(5, 1)).toEqual([]);
    expect(axisTicks(Number.NaN, 1)).toEqual([]);
    expect(axisTicks(1, Number.POSITIVE_INFINITY)).toEqual([]);
  });

  it('collapses an all-equal domain to a single tick', () => {
    expect(axisTicks(7, 7)).toEqual([7]);
  });

  // A quarter-span step is the common case; assert the shape, not exact values.
  it('hits all four quarters of a 0..100 domain', () => {
    expect(axisTicks(0, 100)).toEqual([0, 25, 50, 75, 100]);
  });

  it('keeps ticks monotonic and inside the domain across awkward spans', () => {
    const domains: [number, number][] = [
      [3, 7],
      [-5, 5],
      [0.1, 0.9],
      [1_700_000_000, 1_700_003_600], // an hour of epoch seconds
      [0, 7], // span that cannot split into target 5 evenly
    ];
    for (const [min, max] of domains) {
      const ticks = axisTicks(min, max);
      expect(ticks.length).toBeGreaterThanOrEqual(2);
      expect(ticks[0]).toBeGreaterThanOrEqual(min - 1e-9);
      expect(ticks[ticks.length - 1]).toBeLessThanOrEqual(max + 1e-9);
      for (let i = 1; i < ticks.length; i++) {
        expect(ticks[i]).toBeGreaterThan(ticks[i - 1]);
      }
    }
  });
});

// The mounted half: what actually renders, in jsdom.
(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderChart(props: Parameters<typeof TimeSeriesChart>[0]) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root!.render(<TimeSeriesChart {...props} />);
  });
}

function stubSvgWidth(width: number, height: number) {
  // The legend swatches are svgs too; the chart is the one with role="img".
  const svg = container!.querySelector('svg[role="img"]');
  expect(svg).not.toBeNull();
  svg!.getBoundingClientRect = () =>
    ({ left: 0, top: 0, right: width, bottom: height, width, height, x: 0, y: 0, toJSON: () => ({}) }) as DOMRect;
  return svg!;
}

afterEach(() => {
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

describe('TimeSeriesChart (mounted)', () => {
  const twoSeries = [
    {
      name: 'VUs',
      color: 'text-sky-500',
      points: [
        { x: 0, y: 1 },
        { x: 1, y: 2 },
        { x: 2, y: 3 },
        { x: 3, y: 2 },
        { x: 4, y: 1 },
      ],
    },
    {
      name: 'RPS',
      color: 'text-amber-500',
      points: [
        { x: 0, y: 40 },
        { x: 1, y: 50 },
        { x: 2, y: 60 },
        { x: 3, y: 55 },
        { x: 4, y: 45 },
      ],
    },
  ];

  it('renders one path per multi-point series, with the color class on the stroke', async () => {
    await renderChart({ series: twoSeries });

    const paths = container!.querySelectorAll('path[data-series]');
    expect(paths).toHaveLength(2);
    expect(paths[0].getAttribute('data-series')).toBe('VUs');
    expect(paths[0].getAttribute('class')).toContain('text-sky-500');
    expect(paths[1].getAttribute('data-series')).toBe('RPS');
    expect(paths[1].getAttribute('class')).toContain('text-amber-500');
  });

  it('draws a dot instead of a path for a single-point series', async () => {
    await renderChart({ series: [{ name: 'solo', points: [{ x: 5, y: 5 }] }] });

    expect(container!.querySelectorAll('path[data-series]')).toHaveLength(0);
    const dot = container!.querySelector('circle[data-series="solo"]');
    expect(dot).not.toBeNull();
  });

  it('flattens an all-equal series to a horizontal path rather than dividing by zero', async () => {
    await renderChart({
      series: [
        {
          name: 'flat',
          points: [
            { x: 0, y: 7 },
            { x: 1, y: 7 },
            { x: 2, y: 7 },
          ],
        },
      ],
    });

    const d = container!.querySelector('path[data-series="flat"]')?.getAttribute('d') ?? '';
    const ys = Array.from(d.matchAll(/[ML]([\d.]+),([\d.]+)/g)).map((m) => m[2]);
    expect(ys).toHaveLength(3);
    for (const y of ys) {
      expect(y).toBe(ys[0]);
    }
  });

  it('shows every series name in the legend, including empty ones', async () => {
    await renderChart({ series: [...twoSeries, { name: 'absent', points: [] }] });

    const legend = container!.querySelector('[data-testid="chart-legend"]');
    expect(legend?.textContent).toContain('VUs');
    expect(legend?.textContent).toContain('RPS');
    expect(legend?.textContent).toContain('absent');
    // ...but only the non-empty series get paths.
    expect(container!.querySelectorAll('path[data-series]')).toHaveLength(2);
  });

  it('renders an empty-state box instead of axes when there is nothing to plot', async () => {
    await renderChart({ series: [] });

    expect(container!.querySelector('[data-testid="chart-empty"]')).not.toBeNull();
    expect(container!.querySelector('svg[viewBox]')).toBeNull();
  });

  it('scales responsively: width 100% with a viewBox, never a fixed pixel width', async () => {
    await renderChart({ series: twoSeries });

    const svg = container!.querySelector('svg[role="img"]');
    expect(svg?.getAttribute('width')).toBe('100%');
    expect(svg?.getAttribute('viewBox')).toBe('0 0 800 200');
  });

  it('shows a crosshair readout with every series value at the nearest x on pointer move', async () => {
    await renderChart({ series: twoSeries });
    const svg = stubSvgWidth(800, 200);

    // clientX 400 of an 800-wide render = the viewBox middle; with the plot
    // area at x 44..790 that is data x ~1.91, whose nearest sample is x=2.
    await act(async () => {
      svg.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: 400 }));
    });

    const readout = container!.querySelector('[data-testid="chart-readout"]');
    expect(readout).not.toBeNull();
    expect(readout?.textContent).toContain('VUs: 3.00');
    expect(readout?.textContent).toContain('RPS: 60');
    // The hovered x itself is listed first.
    expect(readout?.textContent).toContain('2.00');
  });

  // Touch arrives as the same pointer events (pointerdown); a tap must
  // inspect just like a hover.
  it('also inspects on pointerdown (touch taps)', async () => {
    await renderChart({ series: twoSeries });
    const svg = stubSvgWidth(800, 200);

    await act(async () => {
      svg.dispatchEvent(new MouseEvent('pointerdown', { bubbles: true, clientX: 100 }));
    });

    expect(container!.querySelector('[data-testid="chart-readout"]')).not.toBeNull();
  });

  it('clears the readout when the pointer leaves', async () => {
    await renderChart({ series: twoSeries });
    const svg = stubSvgWidth(800, 200);

    await act(async () => {
      svg.dispatchEvent(new MouseEvent('pointermove', { bubbles: true, clientX: 400 }));
    });
    expect(container!.querySelector('[data-testid="chart-readout"]')).not.toBeNull();

    // React derives enter/leave from native out/over events (there is no
    // direct pointerleave listener), so the leave half is a pointerout with
    // relatedTarget outside the svg -- null means "left the window".
    await act(async () => {
      svg.dispatchEvent(new MouseEvent('pointerout', { bubbles: true, relatedTarget: null }));
    });
    expect(container!.querySelector('[data-testid="chart-readout"]')).toBeNull();
  });
});
