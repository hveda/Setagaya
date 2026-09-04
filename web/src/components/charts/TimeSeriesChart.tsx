// Plain-SVG time-series chart, Sparkline's no-charting-library rule scaled
// up from one polyline to axes, ticks, gridlines, a legend, and a
// pointer-driven crosshair readout. Series colors are Tailwind text-color
// classes rendered through currentColor -- exactly Sparkline's stroke trick
// -- so charts follow the light/dark theme with zero charting dependencies.
//
// Responsiveness is viewBox-based: the drawing uses a fixed logical
// coordinate space (VB_W x height) and the svg itself is width="100%", so
// the container is never assigned a pixel width and the chart scales
// uniformly at any size.
import { useState } from 'react';
import type { PointerEvent as ReactPointerEvent } from 'react';

/** One plotted sample; x is a Unix timestamp in seconds when xType="time". */
export interface TimeSeriesPoint {
  x: number;
  y: number;
}

export interface SeriesSpec {
  /** Legend label; also the data-series attribute on the drawn path. */
  name: string;
  /** Tailwind text-color class for this series (currentColor). Falls back to chartPalette by index. */
  color?: string;
  points: TimeSeriesPoint[];
}

export interface TimeSeriesChartProps {
  series: SeriesSpec[];
  /** Axis caption for the y units, e.g. "ms". */
  yLabel?: string;
  /** ViewBox height in logical units (default 200). */
  height?: number;
  /** "time" formats x tick labels as HH:MM:SS of the Unix-second timestamp. */
  xType?: 'time';
  /** Formats y tick labels and readout values. */
  formatY?: (v: number) => string;
}

/** Default series colors: strong and distinguishable on white and slate-900 alike. */
export const chartPalette = ['text-sky-500', 'text-amber-500', 'text-emerald-500', 'text-rose-500', 'text-violet-500'];

/** The logical drawing width; the svg renders at width="100%", scaled to this. */
const VB_W = 800;
const PAD_LEFT = 44;
const PAD_RIGHT = 10;
const PAD_TOP = 8;
const PAD_BOTTOM = 24;

/**
 * The sample whose x is closest to the given x (ties keep the earlier
 * point) -- the hover math, exported so it can be unit-tested without a
 * pointer. Returns null for an empty series.
 */
export function nearestPoint(points: TimeSeriesPoint[], x: number): TimeSeriesPoint | null {
  let best: TimeSeriesPoint | null = null;
  let bestD = Infinity;
  for (const p of points) {
    const d = Math.abs(p.x - x);
    if (d < bestD) {
      best = p;
      bestD = d;
    }
  }
  return best;
}

/**
 * "Nice" tick values inside [min, max]: steps from {1, 2, 2.5, 5} x 10^k so
 * a quarter-span step lands exactly (0..100 yields 0/25/50/75/100). Ticks
 * are strictly ascending and never leave the domain; a degenerate domain
 * collapses to a single tick, an inverted one to none.
 */
export function axisTicks(min: number, max: number, target = 5): number[] {
  if (!Number.isFinite(min) || !Number.isFinite(max) || max < min) {
    return [];
  }
  if (max === min) {
    return [min];
  }
  const raw = (max - min) / (target - 1);
  const mag = Math.pow(10, Math.floor(Math.log10(raw)));
  const norm = raw / mag;
  const step = (norm > 5 ? 10 : norm > 2.5 ? 5 : norm > 2 ? 2.5 : norm > 1 ? 2 : 1) * mag;
  const first = Math.ceil(min / step - 1e-9) * step;
  const ticks: number[] = [];
  for (let i = 0; i < 64; i++) {
    const v = first + i * step;
    if (v > max + step * 1e-9) {
      break;
    }
    ticks.push(v);
  }
  return ticks;
}

function defaultFormatY(v: number): string {
  const a = Math.abs(v);
  if (a >= 10000) {
    return `${(v / 1000).toFixed(0)}k`;
  }
  if (a >= 100) {
    return v.toFixed(0);
  }
  if (a >= 10) {
    return v.toFixed(1);
  }
  return v.toFixed(2);
}

function formatXLabel(v: number, xType?: 'time'): string {
  if (xType === 'time') {
    return new Date(v * 1000).toLocaleTimeString([], { hour12: false });
  }
  return defaultFormatY(v);
}

function finitePoints(points: TimeSeriesPoint[]): TimeSeriesPoint[] {
  return points.filter((p) => Number.isFinite(p.x) && Number.isFinite(p.y));
}

/**
 * Multi-series time-series chart with a hover crosshair. An empty series
 * array (or every series empty) renders an empty-state box instead of axes.
 */
export default function TimeSeriesChart({ series, yLabel, height = 200, xType, formatY = defaultFormatY }: TimeSeriesChartProps) {
  const [hoverX, setHoverX] = useState<number | null>(null);

  const withColor = series.map((s, i) => ({ ...s, color: s.color ?? chartPalette[i % chartPalette.length] }));
  const plotted = withColor.map((s) => ({ ...s, points: finitePoints(s.points) })).filter((s) => s.points.length > 0);

  if (plotted.length === 0) {
    return (
      <div
        data-testid="chart-empty"
        className="flex items-center justify-center rounded-lg border border-dashed border-slate-300 text-body-sm text-slate-500 dark:border-slate-600 dark:text-slate-400"
        style={{ height }}
      >
        No data to chart.
      </div>
    );
  }

  const xs = plotted.flatMap((s) => s.points.map((p) => p.x));
  const ys = plotted.flatMap((s) => s.points.map((p) => p.y));
  let xMin = Math.min(...xs);
  let xMax = Math.max(...xs);
  // A single x must still land somewhere: widen to +/-0.5 so the dot centers.
  if (xMin === xMax) {
    xMin -= 0.5;
    xMax += 0.5;
  }
  let yMin = Math.min(...ys);
  let yMax = Math.max(...ys);
  if (yMin === yMax) {
    // All-equal series flatten to the vertical center (Sparkline's rule).
    yMin -= 1;
    yMax += 1;
  } else {
    const pad = (yMax - yMin) * 0.08;
    yMin -= pad;
    yMax += pad;
  }

  const plotW = VB_W - PAD_LEFT - PAD_RIGHT;
  const plotH = Math.max(24, height - PAD_TOP - PAD_BOTTOM);
  const xToPx = (x: number) => PAD_LEFT + ((x - xMin) / (xMax - xMin)) * plotW;
  const yToPx = (y: number) => PAD_TOP + (1 - (y - yMin) / (yMax - yMin)) * plotH;
  const lineD = (points: TimeSeriesPoint[]) =>
    points.map((p, i) => `${i === 0 ? 'M' : 'L'}${xToPx(p.x).toFixed(1)},${yToPx(p.y).toFixed(1)}`).join(' ');

  const xTicks = axisTicks(xMin, xMax, 5);
  const yTicks = axisTicks(yMin, yMax, 5);

  // Pointer -> viewBox x. The svg scales uniformly (width 100%, height
  // auto), so the fraction of clientX across the element maps straight onto
  // VB_W. Mouse and touch both arrive as pointer events; touch-action keeps
  // vertical page scroll working while horizontal drags inspect.
  const handleMove = (e: ReactPointerEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) {
      return;
    }
    setHoverX(((e.clientX - rect.left) / rect.width) * VB_W);
  };

  // The crosshair snaps to the nearest sample x across all series, and the
  // readout lists every series' value at (its nearest point to) that x.
  const allPoints = plotted.flatMap((s) => s.points);
  let crosshair: { cx: number; dataX: number; point: TimeSeriesPoint } | null = null;
  if (hoverX !== null) {
    const px = Math.min(Math.max(hoverX, PAD_LEFT), PAD_LEFT + plotW);
    const dataX = xMin + ((px - PAD_LEFT) / plotW) * (xMax - xMin);
    const point = nearestPoint(allPoints, dataX);
    if (point) {
      crosshair = { cx: xToPx(point.x), dataX, point };
    }
  }

  return (
    <div>
      <div className="mb-2 flex flex-wrap gap-x-4 gap-y-1" data-testid="chart-legend">
        {withColor.map((s) => (
          <span key={s.name} className="inline-flex items-center gap-1.5 text-caption text-slate-600 dark:text-slate-300">
            <svg width="14" height="8" viewBox="0 0 14 8" aria-hidden="true" className={s.color}>
              <path d="M0 4 H14" stroke="currentColor" strokeWidth="2" />
            </svg>
            {s.name}
          </span>
        ))}
      </div>
      <svg
        role="img"
        aria-label={`Time series: ${plotted.map((s) => s.name).join(', ')}`}
        viewBox={`0 0 ${VB_W} ${height}`}
        width="100%"
        className="block"
        style={{ touchAction: 'pan-y' }}
        onPointerMove={handleMove}
        onPointerDown={handleMove}
        onPointerLeave={() => setHoverX(null)}
      >
        {yTicks.map((t) => (
          <g key={`y-${t}`}>
            <line x1={PAD_LEFT} y1={yToPx(t)} x2={VB_W - PAD_RIGHT} y2={yToPx(t)} className="stroke-slate-200 dark:stroke-slate-700" strokeWidth="1" />
            <text x={PAD_LEFT - 6} y={yToPx(t) + 3.5} textAnchor="end" fontSize="11" className="fill-slate-500 dark:fill-slate-400">
              {formatY(t)}
            </text>
          </g>
        ))}
        <line x1={PAD_LEFT} y1={PAD_TOP} x2={PAD_LEFT} y2={PAD_TOP + plotH} className="stroke-slate-300 dark:stroke-slate-600" strokeWidth="1" />
        <line
          x1={PAD_LEFT}
          y1={PAD_TOP + plotH}
          x2={VB_W - PAD_RIGHT}
          y2={PAD_TOP + plotH}
          className="stroke-slate-300 dark:stroke-slate-600"
          strokeWidth="1"
        />
        {xTicks.map((t) => (
          <text key={`x-${t}`} x={xToPx(t)} y={PAD_TOP + plotH + 16} textAnchor="middle" fontSize="11" className="fill-slate-500 dark:fill-slate-400">
            {formatXLabel(t, xType)}
          </text>
        ))}
        {yLabel && (
          <text x={4} y={12} fontSize="11" className="fill-slate-400 dark:fill-slate-500">
            {yLabel}
          </text>
        )}
        {plotted.map((s) =>
          s.points.length >= 2 ? (
            <path
              key={s.name}
              d={lineD(s.points)}
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinejoin="round"
              strokeLinecap="round"
              className={s.color}
              data-series={s.name}
            />
          ) : (
            <circle key={s.name} cx={xToPx(s.points[0].x)} cy={yToPx(s.points[0].y)} r="3" fill="currentColor" className={s.color} data-series={s.name} />
          )
        )}
        {crosshair && (
          <g data-testid="chart-readout">
            <line
              x1={crosshair.cx}
              y1={PAD_TOP}
              x2={crosshair.cx}
              y2={PAD_TOP + plotH}
              className="stroke-slate-400 dark:stroke-slate-500"
              strokeWidth="1"
              strokeDasharray="3 3"
            />
            {plotted.map((s) => {
              const p = nearestPoint(s.points, crosshair!.dataX);
              return p ? <circle key={s.name} cx={xToPx(p.x)} cy={yToPx(p.y)} r="3" fill="currentColor" className={s.color} /> : null;
            })}
            {(() => {
              const lines = [
                formatXLabel(crosshair!.point.x, xType),
                ...plotted.map((s) => {
                  const p = nearestPoint(s.points, crosshair!.dataX);
                  return `${s.name}: ${p ? formatY(p.y) : '—'}`;
                }),
              ];
              const boxW = Math.max(...lines.map((l) => l.length)) * 6.4 + 12;
              const boxH = lines.length * 14 + 8;
              const tx = crosshair!.cx + 10 + boxW > VB_W - PAD_RIGHT ? crosshair!.cx - 10 - boxW : crosshair!.cx + 10;
              return (
                <g transform={`translate(${tx.toFixed(1)}, ${PAD_TOP + 2})`}>
                  <rect width={boxW.toFixed(1)} height={boxH} rx="4" className="fill-white stroke-slate-200 dark:fill-slate-800 dark:stroke-slate-700" strokeWidth="1" />
                  {lines.map((l, i) => (
                    <text key={i} x="6" y={14 + i * 14} fontSize="11" className={i === 0 ? 'fill-slate-400 dark:fill-slate-500' : 'fill-slate-700 dark:fill-slate-200'}>
                      {l}
                    </text>
                  ))}
                </g>
              );
            })()}
          </g>
        )}
      </svg>
    </div>
  );
}
