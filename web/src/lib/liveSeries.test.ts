import { describe, expect, it } from 'vitest';
import { liveSeries, maxBuckets, type ReceivedMetric } from './liveSeries';
import type { EngineMetric } from '../api/status';

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

// A stable epoch base so fixtures read as "second N, plus ms jitter".
const base = 1_700_000_000_000;
const at = (second: number, ms = 0) => base + second * 1000 + ms;
const event = (second: number, over: Partial<EngineMetric> = {}, ms = 0): ReceivedMetric => ({
  receivedAt: at(second, ms),
  metric: metric(over),
});

describe('liveSeries', () => {
  it('returns an empty array for no events (idle stream)', () => {
    expect(liveSeries([])).toEqual([]);
  });

  it('single event forms one bucket with every percentile at its latency', () => {
    const got = liveSeries([event(0, { threads: 4, latency: 0.25 })]);
    expect(got).toEqual([{ t: 0, vus: 4, rps: 1, errPct: 0, p50: 0.25, p95: 0.25, p99: 0.25 }]);
  });

  it('events within the same second share a bucket regardless of ms jitter', () => {
    // t is relative to the earliest event, so the bucket at absolute
    // second 3 is t=0: only one event kind exists in this fixture.
    const got = liveSeries([event(3, { threads: 2 }, 0), event(3, { threads: 5 }, 400), event(3, { threads: 3 }, 999)]);
    expect(got).toHaveLength(1);
    expect(got[0]).toMatchObject({ t: 0, rps: 3, vus: 5 });
  });

  it('buckets by floor of (receivedAt - first receipt), keeping empty seconds as gaps', () => {
    // Events at absolute seconds 1, 3, 5: relative to the earliest (second
    // 1) the buckets are 0, 2, 4 -- second 2 and 4 recorded nothing and
    // stay absent rather than being zero-filled.
    const got = liveSeries([event(1), event(3), event(5)]);
    expect(got.map((p) => p.t)).toEqual([0, 2, 4]);
    expect(got.every((p) => p.rps === 1)).toBe(true);
  });

  it('anchors on the earliest receivedAt when events arrive out of order', () => {
    // The second-5 event arrives FIRST; without a min-anchor its bucket
    // index would be negative.
    const got = liveSeries([event(5), event(0), event(2)]);
    expect(got.map((p) => p.t)).toEqual([0, 2, 5]);
  });

  it('aggregates mixed labels into the same bucket: max threads, summed count', () => {
    const got = liveSeries([
      event(0, { label: 'login', threads: 3 }),
      event(0, { label: 'search', threads: 7 }),
    ]);
    expect(got).toHaveLength(1);
    expect(got[0]).toMatchObject({ vus: 7, rps: 2 });
  });

  it('counts failures by the stream\'s "200"/"500" status convention', () => {
    const got = liveSeries([event(0), event(0, { status: '500' }), event(0, { status: '500' })]);
    expect(got[0].errPct).toBeCloseTo((2 / 3) * 100, 10);
  });

  it('computes nearest-rank percentiles over the bucket\'s latencies', () => {
    // Sorted: [0.1 0.2 0.3 0.4 0.5]. nearest-rank p50 = ceil(0.5*5)=3rd -> 0.3;
    // p95 = ceil(4.75)=5th -> 0.5; p99 clamps to the 5th -> 0.5.
    const got = liveSeries([
      event(0, { latency: 0.3 }),
      event(0, { latency: 0.5 }),
      event(0, { latency: 0.1 }),
      event(0, { latency: 0.4 }),
      event(0, { latency: 0.2 }),
    ]);
    expect(got[0].p50).toBe(0.3);
    expect(got[0].p95).toBe(0.5);
    expect(got[0].p99).toBe(0.5);
  });

  it('keeps exactly the last maxBuckets buckets: 61 seconds drop the oldest', () => {
    const events = Array.from({ length: 61 }, (_, s) => event(s));
    const got = liveSeries(events);
    expect(got).toHaveLength(maxBuckets);
    // The oldest bucket (t=0) is gone; t values keep their run-relative
    // meaning rather than being re-based.
    expect(got.map((p) => p.t)).toEqual(Array.from({ length: 60 }, (_, i) => i + 1));
  });

  it('keeps all 60 buckets when exactly 60 arrive', () => {
    const events = Array.from({ length: 60 }, (_, s) => event(s));
    expect(liveSeries(events).map((p) => p.t)).toEqual(Array.from({ length: 60 }, (_, i) => i));
  });
});
