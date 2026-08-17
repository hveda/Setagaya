// Plain-SVG sparkline (phase 13's no-charting-library rule): a single
// polyline over the value series plus an end dot -- no axes, no ticks; the
// table beside it carries the numbers, this carries the shape. Stroke uses
// currentColor so Tailwind's text color classes control it.
export interface SparkPoint {
  x: number;
  y: number;
}

/**
 * Maps values (display order: left = oldest) to SVG points across
 * [padding, width - padding] x [padding, height - padding]. All-equal
 * series (max === min) flatten to the vertical center rather than dividing
 * by zero; a single value centers horizontally too.
 */
export function toSparklinePoints(values: number[], width: number, height: number, padding = 2): SparkPoint[] {
  if (values.length === 0) {
    return [];
  }
  const min = Math.min(...values);
  const max = Math.max(...values);
  const inner = height - padding * 2;
  const yFor = (v: number) => (max === min ? padding + inner / 2 : padding + inner * (1 - (v - min) / (max - min)));
  const span = width - padding * 2;
  return values.map((v, i) => ({
    x: values.length === 1 ? width / 2 : padding + (span * i) / (values.length - 1),
    y: yFor(v),
  }));
}

export interface SparklineProps {
  /** Values in display order: index 0 renders leftmost (oldest). */
  values: number[];
  /** Names the series for screen readers (role="img" carries it). */
  ariaLabel: string;
  width?: number;
  height?: number;
  className?: string;
}

export default function Sparkline({ values, ariaLabel, width = 160, height = 32, className = '' }: SparklineProps) {
  const points = toSparklinePoints(values, width, height);
  if (points.length === 0) {
    return null;
  }
  const d = points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ');
  const last = points[points.length - 1];
  return (
    <svg
      role="img"
      aria-label={ariaLabel}
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={`shrink-0 ${className}`}
    >
      <path d={d} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" strokeLinecap="round" />
      <circle cx={last.x} cy={last.y} r="2" fill="currentColor" />
    </svg>
  );
}
