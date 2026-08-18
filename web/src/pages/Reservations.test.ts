import { describe, expect, it } from 'vitest';
import { groupByDay, reservationStatus } from './Reservations';
import type { Reservation } from '../api/reservations';

function makeReservation(overrides: Partial<Reservation> = {}): Reservation {
  return {
    id: 1,
    tenant_id: 1,
    cluster: 'default',
    engine_count: 2,
    start: '2026-08-06T10:00:00Z',
    end: '2026-08-06T11:00:00Z',
    execution_id: 42,
    ...overrides,
  };
}

describe('reservationStatus', () => {
  it('is upcoming when now is before start', () => {
    const r = makeReservation();
    const now = new Date('2026-08-06T09:00:00Z');
    expect(reservationStatus(r, now)).toBe('upcoming');
  });

  it('is active when now is within [start, end)', () => {
    const r = makeReservation();
    const now = new Date('2026-08-06T10:30:00Z');
    expect(reservationStatus(r, now)).toBe('active');
  });

  it('is past once now reaches end', () => {
    const r = makeReservation();
    const now = new Date('2026-08-06T11:00:00Z');
    expect(reservationStatus(r, now)).toBe('past');
  });
});

describe('groupByDay', () => {
  it('groups reservations that share a calendar day and preserves the rest as separate groups', () => {
    const sameDay1 = makeReservation({ id: 1, start: '2026-08-06T12:00:00Z' });
    const sameDay2 = makeReservation({ id: 2, start: '2026-08-06T15:00:00Z' });
    const otherDay = makeReservation({ id: 3, start: '2026-08-10T12:00:00Z' });

    const groups = groupByDay([sameDay1, sameDay2, otherDay]);

    expect(groups).toHaveLength(2);
    const [firstDay, firstItems] = groups[0];
    const [secondDay, secondItems] = groups[1];
    expect(firstItems.map((r) => r.id)).toEqual([1, 2]);
    expect(secondItems.map((r) => r.id)).toEqual([3]);
    expect(firstDay).not.toBe(secondDay);
  });

  it('returns no groups for an empty list', () => {
    expect(groupByDay([])).toEqual([]);
  });
});
