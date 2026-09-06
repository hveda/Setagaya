import { describe, expect, it, vi, afterEach } from 'vitest';
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import newTestSource from './NewTest.tsx?raw';
import NewTest from './NewTest';
import { SessionProvider } from '../hooks/useSession';
import { buildConfig, type NewTestForm } from '../lib/newTestFlow';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

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

// The mounted flow (Execution.live.test.tsx's createRoot + act pattern):
// every fetch is stubbed per URL, the PUT to /executions/{id}/config is
// captured, and the payload is compared against buildConfig's output --
// the R9 wire contract the stage editor must keep byte-identical.
const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });

let puts: Array<{ url: string; body: string }> = [];
let container: HTMLDivElement | null = null;
let root: Root | null = null;

async function renderNewTest() {
  container = document.createElement('div');
  document.body.appendChild(container);
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? 'GET';
      if (url.endsWith('/api/me')) {
        return json({ subject: 'demo:a', name: 'a', email: '', global_roles: [], tenants: {}, permissions: { '*': ['*'] }, demo: true });
      }
      if (method === 'GET' && url.endsWith('/api/projects')) {
        return json([]);
      }
      if (method === 'POST' && url.endsWith('/api/projects')) {
        return json({ id: 1, name: 'tests-checkout-smoke' });
      }
      if (method === 'POST' && url.endsWith('/api/scenarios')) {
        return json({ id: 42 });
      }
      if (method === 'POST' && url.endsWith('/api/executions')) {
        return json({ id: 9 });
      }
      if (method === 'PUT' && url.endsWith('/api/scenarios/42/requests')) {
        return json({});
      }
      if (method === 'PUT' && url.endsWith('/api/executions/9/config')) {
        puts.push({ url, body: String(init?.body) });
        return json({});
      }
      return json({ message: `no stub for ${method} ${url}` }, 500);
    }),
  );
  root = createRoot(container);
  await act(async () => {
    root!.render(
      <MemoryRouter initialEntries={['/executions/new']}>
        <SessionProvider>
          <Routes>
            <Route path="/executions/new" element={<NewTest />} />
          </Routes>
        </SessionProvider>
      </MemoryRouter>,
    );
  });
  await act(async () => {});
}

afterEach(() => {
  vi.unstubAllGlobals();
  const r = root;
  if (r !== null && container !== null) {
    act(() => {
      r.unmount();
    });
  }
  container?.remove();
  container = null;
  root = null;
  puts = [];
});

/** Native value setter + input event (React's tracker ignores plain writes). */
async function type(el: HTMLInputElement | HTMLTextAreaElement, value: string) {
  const proto = el instanceof HTMLTextAreaElement ? HTMLTextAreaElement.prototype : HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')!.set!;
  await act(async () => {
    setter.call(el, value);
    el.dispatchEvent(new Event('input', { bubbles: true }));
  });
}

async function click(el: Element) {
  await act(async () => {
    el.dispatchEvent(new MouseEvent('click', { bubbles: true }));
  });
}

/** The page's own defaults (NewTest's initial form), with the typed identity fields. */
function submittedForm(name: string, targetUrl: string): NewTestForm {
  return {
    name,
    targetUrl,
    method: 'GET',
    headers: [],
    concurrency: 50,
    engines: 2,
    rampup: 30,
    duration: 300,
    engine: 'jmeter',
  };
}

/** Fills name + target URL and clicks Create, flushing the five-call flow. */
async function fillAndSubmit() {
  await type(container!.querySelector('input[placeholder="checkout-smoke"]') as HTMLInputElement, 'checkout-smoke');
  await type(container!.querySelector('input[placeholder="http://checkout.svc"]') as HTMLInputElement, 'http://checkout.svc');
  await click(container!.querySelector('[data-testid="create-test"]')!);
}

describe('NewTest step 5 via StageEditor (mounted flow)', () => {
  it('shows the stage editor as the step-5 surface (seeded from the form defaults)', async () => {
    await renderNewTest();
    expect(container!.querySelector('[data-testid="stage-editor"]')).not.toBeNull();
    const concurrency = container!.querySelector('[aria-label="stage 1 concurrency"]') as HTMLInputElement;
    expect(concurrency.value).toBe('50');
    expect((container!.querySelector('[aria-label="stage 1 duration"]') as HTMLInputElement).value).toBe('300');
  });

  it('reaches PUT /executions/{id}/config with a payload byte-equal to buildConfig', async () => {
    await renderNewTest();
    await fillAndSubmit();
    await act(async () => {});

    expect(puts).toHaveLength(1);
    expect(puts[0].url).toContain('/api/executions/9/config');
    const expected = buildConfig(submittedForm('checkout-smoke', 'http://checkout.svc'), 42);
    expected.project_id = 1;
    expected.execution_id = 9;
    // Byte equality: key order and omitted keys (no throughput, no
    // csv_split) are the contract, not just the parsed shape.
    expect(puts[0].body).toBe(JSON.stringify(expected));
  });

  it('submits every edited stage row, each bound to the created scenario', async () => {
    await renderNewTest();
    await type(container!.querySelector('input[placeholder="checkout-smoke"]') as HTMLInputElement, 'checkout-smoke');
    await type(container!.querySelector('input[placeholder="http://checkout.svc"]') as HTMLInputElement, 'http://checkout.svc');
    await click(container!.querySelector('[aria-label="add stage"]')!);
    await type(container!.querySelector('[aria-label="stage 2 concurrency"]') as HTMLInputElement, '25');
    await type(container!.querySelector('[aria-label="stage 2 throughput"]') as HTMLInputElement, '120');
    await click(container!.querySelector('[data-testid="create-test"]')!);
    await act(async () => {});

    const sent = JSON.parse(puts[0].body) as {
      tests: Array<{ name: string; scenario_id: number; concurrency: number; throughput?: number }>;
    };
    expect(sent.tests).toHaveLength(2);
    expect(sent.tests[0]).toEqual({ name: 'checkout-smoke', scenario_id: 42, concurrency: 50, rampup: 30, engines: 2, duration: 300 });
    expect(sent.tests[1].concurrency).toBe(25);
    expect(sent.tests[1].throughput).toBe(120);
    expect(sent.tests[1].scenario_id).toBe(42);
  });
});

// Phase 24: a step failing with the structured details envelope surfaces
// the hint/numbers under the message line; a message-only failure renders
// exactly as before. renderNewTest's stub is swapped before submit because
// the flow fetches at click time.
describe('NewTest action-error details (mounted)', () => {
  it('surfaces the hint code line and numbers when the failing step carried details', async () => {
    await renderNewTest();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? 'GET';
        if (method === 'GET' && url.endsWith('/api/projects')) {
          return json([]);
        }
        if (method === 'POST' && url.endsWith('/api/projects')) {
          return json({ id: 1, name: 'tests-checkout-smoke' });
        }
        if (method === 'POST' && url.endsWith('/api/scenarios')) {
          return json({ id: 42 });
        }
        if (method === 'POST' && url.endsWith('/api/executions')) {
          return json({ id: 9 });
        }
        if (method === 'PUT' && url.endsWith('/api/scenarios/42/requests')) {
          return json({});
        }
        if (method === 'PUT' && url.endsWith('/api/executions/9/config')) {
          return json(
            {
              message: 'reservation would exceed tenant quota',
              details: { requested: 2, used: 0, ceiling: 1, hint: 'PUT /api/tenants/{tenant_id}/quota ceiling=1' },
            },
            429,
          );
        }
        return json({ message: `no stub for ${method} ${url}` }, 500);
      }),
    );

    await fillAndSubmit();
    await act(async () => {});

    expect(container!.querySelector('[role="alert"]')?.textContent).toContain(
      'reservation would exceed tenant quota',
    );
    const details = container!.querySelector('[data-testid="action-error-details"]');
    expect(details?.querySelector('code')?.textContent).toBe('PUT /api/tenants/{tenant_id}/quota ceiling=1');
    expect(details?.textContent).toContain('used 0 / ceiling 1 — requested 2');
    // The flow stopped at the failing step: no navigation away from the form.
    expect(puts).toHaveLength(0);
  });

  it('message-only failures keep the single alert line (no details node)', async () => {
    await renderNewTest();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const url = String(input);
        const method = init?.method ?? 'GET';
        if (method === 'GET' && url.endsWith('/api/projects')) {
          return json([]);
        }
        if (method === 'POST' && url.endsWith('/api/projects')) {
          return json({ id: 1, name: 'tests-checkout-smoke' });
        }
        if (method === 'POST' && url.endsWith('/api/scenarios')) {
          return json({ message: 'scenario name already in use' }, 409);
        }
        return json({ message: `no stub for ${method} ${url}` }, 500);
      }),
    );

    await fillAndSubmit();
    await act(async () => {});

    expect(container!.querySelector('[role="alert"]')?.textContent).toContain('scenario name already in use');
    expect(container!.querySelector('[data-testid="action-error-details"]')).toBeNull();
  });
});
