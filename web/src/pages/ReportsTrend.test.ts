import { describe, expect, it } from 'vitest';
import { sortSignatureGroups } from './ReportsTrend';
import type { SignatureBreakdown } from '../api/trends';

function makeGroup(overrides: Partial<SignatureBreakdown> = {}): SignatureBreakdown {
  return { key: 'GET /', total_count: 10, rows: [], ...overrides };
}

describe('sortSignatureGroups', () => {
  it('orders groups biggest-total-first', () => {
    const got = sortSignatureGroups([
      makeGroup({ key: 'GET /orders', total_count: 5 }),
      makeGroup({ key: 'GET /', total_count: 40 }),
      makeGroup({ key: 'POST /cart', total_count: 12 }),
    ]);

    expect(got.map((g) => g.key)).toEqual(['GET /', 'POST /cart', 'GET /orders']);
  });

  it('breaks total ties by key alphabetically so rendering is deterministic', () => {
    const got = sortSignatureGroups([
      makeGroup({ key: 'b', total_count: 10 }),
      makeGroup({ key: 'a', total_count: 10 }),
    ]);

    expect(got.map((g) => g.key)).toEqual(['a', 'b']);
  });

  it('orders rows within a group biggest-first', () => {
    const got = sortSignatureGroups([
      makeGroup({
        rows: [
          { label: 'GET /', response_code: '500', side: 'target', total_count: 3, run_count: 2 },
          { label: 'GET /', response_code: '503', side: 'target', total_count: 9, run_count: 4 },
        ],
      }),
    ]);

    expect(got[0].rows.map((r) => r.response_code)).toEqual(['503', '500']);
  });

  it('does not mutate the input', () => {
    const input = [
      makeGroup({ key: 'b', total_count: 10, rows: [{ label: 'x', side: 'target', total_count: 1, run_count: 1 }] }),
      makeGroup({ key: 'a', total_count: 20 }),
    ];
    const snapshot = JSON.parse(JSON.stringify(input)) as SignatureBreakdown[];

    sortSignatureGroups(input);

    expect(input).toEqual(snapshot);
  });
});
