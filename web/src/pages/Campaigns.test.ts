import { describe, expect, it } from 'vitest';
import {
  campaignStatus,
  comparisonStatusClasses,
  comparisonStatusLabels,
  comparisonTransition,
  parseBaselineId,
  serviceStatus,
} from './Campaigns';
import type { Campaign, ServiceVerdict } from '../api/campaigns';
import type { ComparisonStatus } from '../api/comparison';

function makeCampaign(overrides: Partial<Campaign> = {}): Campaign {
  return {
    id: 1,
    name: 'Launch Readiness',
    tenant_id: 7,
    window_start: '2026-08-06T10:00:00Z',
    window_end: '2026-08-06T11:00:00Z',
    services: [],
    active: false,
    ...overrides,
  };
}

describe('campaignStatus', () => {
  it('is upcoming when now is before the window opens', () => {
    const c = makeCampaign();
    expect(campaignStatus(c, new Date('2026-08-06T09:00:00Z'))).toBe('upcoming');
  });

  it('is active when now is within [start, end)', () => {
    const c = makeCampaign();
    expect(campaignStatus(c, new Date('2026-08-06T10:30:00Z'))).toBe('active');
  });

  it('is ended once now reaches the window end', () => {
    const c = makeCampaign();
    expect(campaignStatus(c, new Date('2026-08-06T11:00:00Z'))).toBe('ended');
  });

  // aborted_at wins even mid-window -- matches campaign.Campaign.IsActive,
  // which treats an abort as immediately overriding the window check.
  it('is aborted whenever aborted_at is set, even mid-window', () => {
    const c = makeCampaign({ aborted_at: '2026-08-06T10:15:00Z' });
    expect(campaignStatus(c, new Date('2026-08-06T10:30:00Z'))).toBe('aborted');
  });
});

function makeServiceVerdict(overrides: Partial<ServiceVerdict> = {}): ServiceVerdict {
  return { project_id: 1, execution_id: 10, has_report: false, ...overrides };
}

describe('serviceStatus', () => {
  it('is pending when the service has no report yet', () => {
    expect(serviceStatus(makeServiceVerdict())).toBe('pending');
  });

  it('reflects the report outcome once one exists', () => {
    expect(serviceStatus(makeServiceVerdict({ has_report: true, outcome: 'passed' }))).toBe('passed');
    expect(serviceStatus(makeServiceVerdict({ has_report: true, outcome: 'failed' }))).toBe('failed');
  });

  // has_report:true with no outcome shouldn't happen per the backend
  // contract (outcome is only omitted when has_report is false), but
  // pending is the safe fallback rather than crashing on an unknown badge.
  it('falls back to pending if has_report is true but outcome is missing', () => {
    expect(serviceStatus(makeServiceVerdict({ has_report: true }))).toBe('pending');
  });
});

const allStatuses: ComparisonStatus[] = [
  'improved',
  'regressed',
  'newly_at_risk',
  'still_at_risk',
  'steady',
  'new',
  'dropped',
];

describe('comparison status mapping', () => {
  // Table-test the full classification set: every wire status needs a
  // display label (underscores gone) and chip classes, or the badge
  // renders undefined.
  it.each(allStatuses)('labels and styles %s', (status) => {
    expect(comparisonStatusLabels[status]).toBeTruthy();
    expect(comparisonStatusLabels[status]).not.toMatch(/_/);
    expect(comparisonStatusClasses[status]).toBeTruthy();
  });

  it('covers exactly the seven classifications', () => {
    expect(Object.keys(comparisonStatusLabels).sort()).toEqual([...allStatuses].sort());
  });

  it('humanizes the snake_case risk statuses', () => {
    expect(comparisonStatusLabels.newly_at_risk).toBe('newly at risk');
    expect(comparisonStatusLabels.still_at_risk).toBe('still at risk');
  });
});

describe('comparisonTransition', () => {
  // The transition caption mirrors the classification's definition
  // (domain/campaign/compare.go): improved/regressed are verdict flips,
  // steady/still_at_risk hold, new/dropped are participation changes.
  it.each([
    ['improved', 'no-go → go'],
    ['regressed', 'go → no-go'],
    ['steady', 'go → go'],
    ['still_at_risk', 'no-go → no-go'],
    ['newly_at_risk', 'new: no-go'],
    ['new', 'new: go'],
    ['dropped', 'left this campaign'],
  ] as [ComparisonStatus, string][])('summarizes %s', (status, expected) => {
    expect(comparisonTransition(status)).toBe(expected);
  });
});

describe('parseBaselineId', () => {
  it('accepts a positive campaign id', () => {
    expect(parseBaselineId('4')).toBe(4);
    expect(parseBaselineId(' 12 ')).toBe(12);
  });

  it('treats empty (or blank) input as the default resolution', () => {
    expect(parseBaselineId('')).toBe('empty');
    expect(parseBaselineId('   ')).toBe('empty');
  });

  // Number('') is 0, and 0 is not a valid campaign id -- blank input must
  // not slip through as an override.
  it('rejects zero, negatives, fractions, and junk', () => {
    expect(parseBaselineId('0')).toBe('invalid');
    expect(parseBaselineId('-3')).toBe('invalid');
    expect(parseBaselineId('1.5')).toBe('invalid');
    expect(parseBaselineId('abc')).toBe('invalid');
  });
});
