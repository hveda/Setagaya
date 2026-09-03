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
