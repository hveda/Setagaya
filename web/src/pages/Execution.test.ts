import { describe, expect, it } from 'vitest';
import { engineShortfall, outcomeBadge, phaseControls, shortTime } from './Execution';

// R2's control matrix: which actions the hub offers per phase, and when
// trigger unlocks (only once every engine is reachable). This is the
// spec-critical surface, asserted without a DOM.
describe('phaseControls', () => {
  it('idle: only deploy is enabled', () => {
    const c = Object.fromEntries(phaseControls('idle', false).map((x) => [x.action, x.enabled]));
    expect(c).toEqual({ deploy: true, trigger: false, stop: false, purge: false });
  });

  it('deployed: trigger unlocks only when engines reachable; purge available', () => {
    const blocked = Object.fromEntries(phaseControls('deployed', false).map((x) => [x.action, x.enabled]));
    expect(blocked.trigger).toBe(false);
    expect(blocked.purge).toBe(true);

    const ready = Object.fromEntries(phaseControls('deployed', true).map((x) => [x.action, x.enabled]));
    expect(ready.trigger).toBe(true);
    expect(ready.deploy).toBe(false);
  });

  it('running: stop and purge enabled, everything else locked', () => {
    const c = Object.fromEntries(phaseControls('running', true).map((x) => [x.action, x.enabled]));
    expect(c).toEqual({ deploy: false, trigger: false, stop: true, purge: true });
  });

  it('null phase (not loaded): nothing enabled', () => {
    const c = Object.fromEntries(phaseControls(null, false).map((x) => [x.action, x.enabled]));
    expect(Object.values(c).every((v) => v === false)).toBe(true);
  });
});

describe('engineShortfall', () => {
  it('floors at zero (terminating engine can briefly over-report)', () => {
    expect(engineShortfall({ engines: 3, engines_deployed: 3 })).toBe(0);
    expect(engineShortfall({ engines: 3, engines_deployed: 4 })).toBe(0);
    expect(engineShortfall({ engines: 3, engines_deployed: 1 })).toBe(2);
  });
});

// R3: report rows and log tails.
describe('outcomeBadge', () => {
  it('maps every outcome to a non-empty class string', () => {
    for (const outcome of ['passed', 'failed', 'aborted', 'error'] as const) {
      expect(outcomeBadge(outcome)).toMatch(/bg-/);
    }
    // passed and failed must color differently -- the whole point of a badge.
    expect(outcomeBadge('passed')).not.toBe(outcomeBadge('failed'));
    expect(outcomeBadge('aborted')).not.toBe(outcomeBadge('error'));
  });
});

describe('shortTime', () => {
  it('formats an ISO timestamp as MM-DD HH:mm', () => {
    expect(shortTime('2026-09-02T10:05:00Z')).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/);
  });

  it('passes through garbage unchanged (no crash on odd server data)', () => {
    expect(shortTime('not-a-date')).toBe('not-a-date');
  });
});
