import { describe, expect, it } from 'vitest';
import appSource from './App.tsx?raw';

// R1's route contract, pinned against the router's source: /status must
// redirect to /executions (existing bookmark, new home) and the execution
// list must be mounted at /executions. Reading the mounted component through
// the router in jsdom drags every page's data fetching with it; the ?raw
// import keeps this test to the wiring itself.
describe('App routes (R1)', () => {
  it('redirects /status to /executions', () => {
    const statusRoute = appSource.split('\n').find((l) => l.includes('path="/status"'));
    expect(statusRoute).toBeDefined();
    expect(statusRoute).toContain('Navigate');
    expect(statusRoute).toContain('to="/executions"');
  });

  it('mounts the executions list at /executions', () => {
    const execRoute = appSource.split('\n').find((l) => l.includes('path="/executions"'));
    expect(execRoute).toBeDefined();
    expect(execRoute).toContain('Executions');
  });

  it('does not mount LiveStatus at /status anymore', () => {
    const statusRoute = appSource.split('\n').find((l) => l.includes('path="/status"'));
    expect(statusRoute).not.toContain('LiveStatus');
  });
});
