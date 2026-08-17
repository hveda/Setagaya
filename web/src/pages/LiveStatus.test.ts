import { describe, expect, it } from 'vitest';
import { engineShortfall, summarize } from './LiveStatus';
import type { ReceivedMetric } from './LiveStatus';
import type { EngineMetric } from '../api/status';

function makeMetric(overrides: Partial<EngineMetric> = {}): EngineMetric {
  return {
    threads: 10,
    latency: 0.25,
    label: 'GET /',
    status: '200',
    raw: '',
    execution_id: '1',
    scenario_id: '1',
    engine_id: '0',
    run_id: '1',
    ...overrides,
  };
}

describe('summarize', () => {
  it('reports zeroed stats and null latency for an empty window', () => {
    expect(summarize([])).toEqual({ throughput: 0, errorRate: 0, latencySeconds: null });
  });

  it('derives throughput from event count over the 10s window and uses the latest latency', () => {
    const events: ReceivedMetric[] = [
      { receivedAt: 1000, metric: makeMetric({ latency: 0.1 }) },
      { receivedAt: 2000, metric: makeMetric({ latency: 0.2 }) },
      { receivedAt: 3000, metric: makeMetric({ latency: 0.3 }) },
    ];
    const stats = summarize(events);
    expect(stats.throughput).toBeCloseTo(0.3, 5);
    expect(stats.latencySeconds).toBe(0.3);
    expect(stats.errorRate).toBe(0);
  });

  it('computes error rate as the fraction of non-200 events', () => {
    const events: ReceivedMetric[] = [
      { receivedAt: 1000, metric: makeMetric({ status: '200' }) },
      { receivedAt: 2000, metric: makeMetric({ status: '500' }) },
      { receivedAt: 3000, metric: makeMetric({ status: '500' }) },
      { receivedAt: 4000, metric: makeMetric({ status: '200' }) },
    ];
    expect(summarize(events).errorRate).toBeCloseTo(0.5, 5);
  });
});

describe('engineShortfall', () => {
  it('is zero when every wanted engine is deployed', () => {
    expect(engineShortfall({ engines: 4, engines_deployed: 4 })).toBe(0);
  });

  it('counts engines still missing while a scenario scales up', () => {
    expect(engineShortfall({ engines: 4, engines_deployed: 1 })).toBe(3);
  });

  it('never goes negative when a terminating engine briefly over-reports', () => {
    expect(engineShortfall({ engines: 4, engines_deployed: 5 })).toBe(0);
  });
});
