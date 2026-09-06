// ActionErrorDetails renders the phase-24 details envelope under an action
// error's message line: hint as a <code> chip, quota numbers as arithmetic.
// CopyLink.test.tsx's createRoot + act pattern.
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it } from 'vitest';
import ActionErrorDetails from './ActionErrorDetails';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

let container: HTMLDivElement | null = null;
let root: Root | null = null;

function render(details: Record<string, unknown> | null) {
  container = document.createElement('div');
  document.body.appendChild(container);
  root = createRoot(container);
  act(() => {
    root!.render(<ActionErrorDetails details={details} />);
  });
}

afterEach(() => {
  const r = root;
  if (r !== null && container !== null) {
    act(() => {
      r.unmount();
    });
  }
  container?.remove();
  container = null;
  root = null;
});

describe('ActionErrorDetails', () => {
  it('renders nothing without details (plain errors look exactly as before)', () => {
    render(null);
    expect(container!.textContent).toBe('');
  });

  it('renders the hint as a code line plus the quota numbers line', () => {
    render({ hint: 'PUT /api/tenants/{tenant_id}/quota ceiling=1', used: 0, ceiling: 1, requested: 2 });
    expect(container!.querySelector('code')?.textContent).toBe('PUT /api/tenants/{tenant_id}/quota ceiling=1');
    expect(container!.textContent).toContain('used 0 / ceiling 1 — requested 2');
  });

  it('renders a hint-only envelope (the 409 case) without a numbers line', () => {
    render({ hint: 'purge the execution and redeploy before triggering', orphaned_completions: 3 });
    expect(container!.querySelector('code')?.textContent).toBe('purge the execution and redeploy before triggering');
    expect(container!.textContent).not.toContain('ceiling');
  });

  it('tolerates missing numbers when ceiling is present', () => {
    render({ ceiling: 4, hint: 'PUT /api/tenants/{tenant_id}/quota ceiling=4' });
    expect(container!.textContent).toContain('used ? / ceiling 4 — requested ?');
  });

  it('ignores non-string hints and non-number fields rather than rendering junk', () => {
    render({ hint: 42, ceiling: 'lots' });
    expect(container!.querySelector('code')).toBeNull();
    expect(container!.textContent).toBe('');
  });
});
