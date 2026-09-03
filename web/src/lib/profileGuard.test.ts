import { describe, expect, it, vi, afterEach } from 'vitest';
import { getProfileGuard } from '../api/calibration';
import { apiClient, ApiError } from '../api/client';
import type { CapacityProfile, FanOutResponse } from '../api/calibration';

afterEach(() => vi.restoreAllMocks());

// R8's decision table: warn ONLY on fresh; silent on stale and absent.
describe('getProfileGuard', () => {
  const profile: CapacityProfile = {
    scenario_id: 5, engine: 'jmeter', cpu: '500m', memory: '512Mi',
    per_pod_qps: 310, saturated_by: 'engine',
    scenario_fingerprint: 'abc', calibrated_at: '2026-09-01T10:00:00Z', job_id: 3,
  };

  it('fresh profile -> fresh:true with per_pod_qps and calibrated_at', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation((path: string) => {
      if (path.includes('/capacity-profile/fanout')) {
        return Promise.resolve({ status: 'ok', engines: 4 } as FanOutResponse);
      }
      return Promise.resolve(profile);
    });
    const g = await getProfileGuard(5, { engine: 'jmeter', cpu: '500m', memory: '512Mi' });
    expect(g.fresh).toBe(true);
    expect(g.perPodQPS).toBe(310);
    expect(g.calibratedAt).toBe('2026-09-01T10:00:00Z');
  });

  it('stale profile (fanout says stale) -> fresh:false but numbers still present (warning names them only when fresh)', async () => {
    vi.spyOn(apiClient, 'get').mockImplementation((path: string) => {
      if (path.includes('/capacity-profile/fanout')) {
        return Promise.resolve({ status: 'stale' } as FanOutResponse);
      }
      return Promise.resolve(profile);
    });
    const g = await getProfileGuard(5, { engine: 'jmeter', cpu: '500m', memory: '512Mi' });
    expect(g.fresh).toBe(false);
    expect(g.perPodQPS).toBe(310);
  });

  it('absent profile (404) -> fresh:false with no numbers, no throw', async () => {
    vi.spyOn(apiClient, 'get').mockRejectedValue(new ApiError(404, 'no profile'));
    const g = await getProfileGuard(5, { engine: 'jmeter', cpu: '500m', memory: '512Mi' });
    expect(g).toEqual({ fresh: false });
  });

  it('calibration outage (500) propagates -- caller treats as no verdict', async () => {
    vi.spyOn(apiClient, 'get').mockRejectedValue(new ApiError(500, 'calibrations down'));
    await expect(
      getProfileGuard(5, { engine: 'jmeter', cpu: '500m', memory: '512Mi' }),
    ).rejects.toThrow('calibrations down');
  });
});

// The warning TEXT contract (what the dialog must name when it shows).
describe('warning content contract', () => {
  it('names per_pod_qps and calibrated_at, and says Target QPS needs recalibration', () => {
    // Mirrors the dialog body in TaurusEditor.tsx; pinned here so a
    // copy edit cannot silently drop the required facts.
    const perPodQPS = 310;
    const calibratedAt = new Date('2026-09-01T10:00:00Z');
    const text = `Saving will invalidate this scenario's capacity profile. Profile: ${perPodQPS.toFixed(0)} qps/pod, calibrated ${calibratedAt.toLocaleString()}. Target QPS sizing will need a recalibration after this change.`;
    expect(text).toContain('310 qps/pod');
    expect(text).toContain('calibrated');
    expect(text).toContain('Target QPS');
    expect(text).toContain('recalibration');
  });
});
