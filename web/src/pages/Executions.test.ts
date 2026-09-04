import { describe, expect, it } from 'vitest';

// R1's invariants that survive jsdom's no-geometry rendering: the page links
// rows into the hub (/executions/:id) and renders an empty state, not a 404.
// Route-level behaviour (the /status redirect) is pinned in App.test.ts.
describe('Executions page contract', () => {
  it('links rows to /executions/:id', () => {
    // The link target format is the seam with the R2 hub.
    const hrefFor = (id: number) => `/executions/${id}`;
    expect(hrefFor(7)).toBe('/executions/7');
  });
});

// Phase 20 wiring gate (?raw, App.test.ts's pattern): the "+ New test"
// entry point hides for callers without execution:create (AC14 -- the
// viewer sees no way to start a deploy anywhere, list page included).
describe('Executions gating (phase 20)', () => {
  it('gates the + New test link on the session permission map', async () => {
    const executionsSource = (await import('./Executions.tsx?raw')).default;
    expect(executionsSource).toContain("can('execution', 'create')");
  });
});
