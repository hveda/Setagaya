import { describe, expect, it } from 'vitest';
import { mobileMenuClasses } from './DashboardLayout';

// Regression guard for the mobile layout bug found live on 2026-08-18: the
// drawer was hidden with `invisible` but left in normal flow, so it still
// reserved its full height. Measured against the deployed SPA at a 390x664
// viewport, <nav> came out 370px tall instead of h-16's 64px and <main>
// started at y=370 on all five pages -- ~306px of empty band, about 46% of
// the viewport, which is what read on a phone as "broken with half hovering
// empty". Positioning the drawer out of flow is the fix, so that is what
// these assert.
describe('mobileMenuClasses', () => {
  // The load-bearing one. A closed drawer that is not absolutely positioned
  // reserves layout height no matter how it is visually hidden -- which is
  // exactly how the bug shipped.
  it('positions the drawer out of document flow in both states', () => {
    for (const isOpen of [true, false]) {
      const classes = mobileMenuClasses(isOpen);
      expect(classes).toContain('absolute');
      expect(classes).toContain('top-full');
    }
  });

  it('hides a closed drawer without letting it swallow taps', () => {
    const closed = mobileMenuClasses(false);
    // invisible/opacity-0 hide it; pointer-events-none stops its links
    // intercepting taps aimed at the burger button underneath, since a
    // closed drawer still overlaps the nav bar.
    expect(closed).toContain('invisible');
    expect(closed).toContain('opacity-0');
    expect(closed).toContain('pointer-events-none');
  });

  // -translate-y-2 (the original) moved it only 8px, leaving it peeking out
  // from behind the bar; -translate-y-full clears it by its own height.
  it('slides a closed drawer fully behind the nav bar', () => {
    expect(mobileMenuClasses(false)).toContain('-translate-y-full');
    expect(mobileMenuClasses(false)).not.toContain('-translate-y-2');
  });

  it('reveals an open drawer', () => {
    const open = mobileMenuClasses(true);
    expect(open).toContain('translate-y-0');
    expect(open).toContain('opacity-100');
    expect(open).not.toContain('invisible');
    expect(open).not.toContain('pointer-events-none');
  });

  // The click-outside handler matches on this class to tell "inside the
  // drawer" from "outside it"; losing it silently breaks tap-away-to-close.
  it('carries the marker class the click-outside handler matches on', () => {
    expect(mobileMenuClasses(true)).toContain('mobile-menu');
  });

  // Desktop keeps its own inline nav links, so the drawer must stay hidden
  // there regardless of the open flag.
  it('stays hidden at md and above', () => {
    expect(mobileMenuClasses(true)).toContain('md:hidden');
    expect(mobileMenuClasses(false)).toContain('md:hidden');
  });
});
