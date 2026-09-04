// The hook is behaviour, not wiring: the 401-branch (picker state), the
// logout flow, and can()'s binding to the live session are what tasks 22-23
// consume, so this test mounts the provider with React's own createRoot +
// act -- the first mounted test in the suite, because until now nothing in
// the SPA had state worth mounting (pages fetch and render once).
import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { createSession } from '../api/session';
import { SessionProvider, resolveMeOutcome, useSession } from './useSession';
import type { SessionContextValue } from './useSession';
import type { SessionInfo } from '../api/session';

(globalThis as Record<string, unknown>).IS_REACT_ACT_ENVIRONMENT = true;

const alice: SessionInfo = {
  subject: 'demo:alice',
  name: 'Alice (service provider admin)',
  email: '',
  global_roles: ['service_provider_admin'],
  tenants: {},
  permissions: { '*': ['*'] },
  demo: true,
};

const bob: SessionInfo = {
  subject: 'demo:bob',
  name: 'Bob (tenant editor)',
  email: '',
  global_roles: [],
  tenants: { '1': ['tenant_editor'] },
  permissions: { execution: ['create', 'delete', 'list', 'read', 'update'] },
  demo: true,
};

/** Routes stubbed fetch by path; records every call in order. */
function stubApi(handlers: Record<string, () => Response>, calls: string[] = []) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    calls.push(`${init?.method ?? 'GET'} ${url}`);
    for (const [path, respond] of Object.entries(handlers)) {
      if (url.endsWith(path)) {
        return respond();
      }
    }
    return new Response(JSON.stringify({ message: `no stub for ${url}` }), { status: 500 });
  });
}

const me200 = (me: SessionInfo) => () =>
  new Response(JSON.stringify(me), { status: 200, headers: { 'Content-Type': 'application/json' } });
const me401 = () =>
  new Response(JSON.stringify({ message: 'unauthenticated' }), { status: 401, headers: { 'Content-Type': 'application/json' } });
const profiles200 = () =>
  new Response(JSON.stringify({ profiles: [{ id: 'alice', name: 'Alice (service provider admin)' }] }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  });

let container: HTMLDivElement | null = null;
let root: Root | null = null;

/** Renders the provider with a probe child that captures the context value. */
async function renderProvider(fetchMock: ReturnType<typeof stubApi>): Promise<{ current: SessionContextValue | null }> {
  container = document.createElement('div');
  document.body.appendChild(container);
  const captured: { current: SessionContextValue | null } = { current: null };
  function Probe() {
    captured.current = useSession();
    return null;
  }
  vi.stubGlobal('fetch', fetchMock);
  root = createRoot(container);
  await act(async () => {
    root!.render(
      <SessionProvider>
        <Probe />
      </SessionProvider>
    );
  });
  return captured;
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
});

describe('resolveMeOutcome', () => {
  it('a 401 is the unauthenticated state, not an error', () => {
    expect(resolveMeOutcome(401, null, 'unauthenticated')).toEqual({ session: null, error: null });
  });

  it('a resolved me is the session', () => {
    expect(resolveMeOutcome(200, bob, '')).toEqual({ session: bob, error: null });
  });

  it('transport and server failures surface as errors', () => {
    expect(resolveMeOutcome(0, null, 'fetch failed')).toEqual({ session: null, error: 'fetch failed' });
    expect(resolveMeOutcome(500, null, 'boom')).toEqual({ session: null, error: 'boom' });
    expect(resolveMeOutcome(500, null, '')).toEqual({ session: null, error: 'request failed with status 500' });
  });
});

describe('SessionProvider', () => {
  it('fetches /api/me on mount and exposes the session', async () => {
    const calls: string[] = [];
    const fetchMock = stubApi({ '/api/me': me200(bob) }, calls);
    const captured = await renderProvider(fetchMock);

    expect(calls).toEqual(['GET /api/me']);
    expect(captured.current?.loading).toBe(false);
    expect(captured.current?.session?.subject).toBe('demo:bob');
    expect(captured.current?.error).toBeNull();
    // Authenticated: no profile fetch happened.
    expect(captured.current?.profiles).toEqual([]);
  });

  it('binds can() to the live session', async () => {
    const captured = await renderProvider(stubApi({ '/api/me': me200(bob) }));
    expect(captured.current?.can('execution', 'create')).toBe(true);
    expect(captured.current?.can('execution', 'delete')).toBe(true);
    expect(captured.current?.can('system', 'admin')).toBe(false);
  });

  it('a 401 means unauthenticated: session null, profiles loaded for the picker', async () => {
    const calls: string[] = [];
    const captured = await renderProvider(
      stubApi({ '/api/me': me401, '/api/session/profiles': profiles200 }, calls)
    );

    expect(captured.current?.session).toBeNull();
    expect(captured.current?.error).toBeNull();
    expect(captured.current?.profiles).toEqual([{ id: 'alice', name: 'Alice (service provider admin)' }]);
    expect(captured.current?.can('execution', 'read')).toBe(false); // fails closed
  });

  it('a failed profile list is reported without breaking the unauthenticated state', async () => {
    const captured = await renderProvider(
      stubApi({
        '/api/me': me401,
        '/api/session/profiles': () =>
          new Response(JSON.stringify({ message: 'demo sessions not configured' }), { status: 404 }),
      })
    );

    expect(captured.current?.session).toBeNull();
    expect(captured.current?.profiles).toEqual([]);
    expect(captured.current?.profilesError).toBe('demo sessions not configured');
  });

  it('logout DELETEs /api/session, then re-resolves identity to unauthenticated', async () => {
    // /api/me answers 200 (Bob) until the logout DELETE, then 401.
    let authenticated = true;
    const calls: string[] = [];
    const captured = await renderProvider(
      stubApi(
        {
          '/api/me': () => (authenticated ? me200(bob)() : me401()),
          '/api/session': () => {
            authenticated = false;
            return new Response(null, { status: 204 });
          },
          '/api/session/profiles': profiles200,
        },
        calls
      )
    );
    expect(captured.current?.session?.subject).toBe('demo:bob');

    await act(async () => {
      await captured.current?.logout();
    });

    expect(calls).toEqual([
      'GET /api/me',
      'DELETE /api/session',
      'GET /api/me', // the re-resolve AC13 rides on
      'GET /api/session/profiles',
    ]);
    expect(captured.current?.session).toBeNull();
    expect(captured.current?.loading).toBe(false);
    expect(captured.current?.can('execution', 'create')).toBe(false);
  });

  it('refresh() flips the app to authenticated after the picker selects a persona', async () => {
    let authenticated = false;
    const captured = await renderProvider(
      stubApi({
        '/api/me': () => (authenticated ? me200(alice)() : me401()),
        '/api/session': () => {
          authenticated = true;
          return new Response(null, { status: 204 });
        },
        '/api/session/profiles': profiles200,
      })
    );
    expect(captured.current?.session).toBeNull();

    // The picker's select (task 22): mint the cookie first, THEN let the
    // app re-ask who it is. refresh() alone cannot authenticate anyone.
    await act(async () => {
      await createSession('alice');
    });
    await act(async () => {
      await captured.current?.refresh();
    });

    expect(captured.current?.session?.subject).toBe('demo:alice');
    expect(captured.current?.profiles).toEqual([]); // picker data cleared once authed
  });
});
