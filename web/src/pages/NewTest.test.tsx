import { describe, expect, it } from 'vitest';
import newTestSource from './NewTest.tsx?raw';

// Wiring test, App.test.ts's ?raw pattern: mounting the whole create flow
// drags five API calls with it, and the gate itself is one line. What this
// pins is that the create control CANNOT render for a caller the server
// would 403 -- AC14's "no Deploy control on any page" includes this one.
describe('NewTest gating (phase 20)', () => {
  it('gates the create control on the session permission map', () => {
    expect(newTestSource).toContain("can('execution', 'create')");
    // The honest alternative rendered instead of the button.
    expect(newTestSource).toContain('no-create-permission');
  });
});
