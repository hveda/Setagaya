import { describe, expect, it } from 'vitest';
import { toSparklinePoints } from './Sparkline';

const W = 160;
const H = 32;
const PAD = 2;

describe('toSparklinePoints', () => {
  it('returns no points for an empty series', () => {
    expect(toSparklinePoints([], W, H)).toEqual([]);
  });

  it('places a single value at the horizontal center', () => {
    const [p] = toSparklinePoints([5], W, H);
    expect(p.x).toBe(W / 2);
    expect(p.y).toBeGreaterThanOrEqual(PAD);
    expect(p.y).toBeLessThanOrEqual(H - PAD);
  });

  // A flat series has max === min; the scale must not divide by zero, and
  // the only sensible shape is a horizontal line at the vertical center.
  it('flattens an all-equal series to the vertical center', () => {
    const points = toSparklinePoints([7, 7, 7, 7], W, H);
    expect(points).toHaveLength(4);
    for (const p of points) {
      expect(p.y).toBe(H / 2);
    }
  });

  it('spreads points across the width, first at the left edge and last at the right', () => {
    const points = toSparklinePoints([1, 2, 3, 4], W, H);
    expect(points[0].x).toBeCloseTo(PAD, 5);
    expect(points[points.length - 1].x).toBeCloseTo(W - PAD, 5);
    // Evenly spaced between.
    for (let i = 1; i < points.length; i++) {
      const step = points[i].x - points[i - 1].x;
      expect(step).toBeCloseTo((W - 2 * PAD) / 3, 5);
    }
  });

  // SVG y grows downward: the smallest value renders lowest (largest y),
  // the highest value renders at the top padding.
  it('maps the max to the top and the min to the bottom', () => {
    const points = toSparklinePoints([1, 3, 2], W, H);
    expect(points[0].y).toBeCloseTo(H - PAD, 5);
    expect(points[1].y).toBeCloseTo(PAD, 5);
    expect(points[2].y).toBeCloseTo(H / 2, 5);
  });
});
