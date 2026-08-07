import { describe, expect, it } from 'vitest';
import { campaignStatus, serviceStatus } from './Campaigns';
import type { Campaign, ServiceVerdict } from '../api/campaigns';

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
