import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from './client';
import { can, createSession, deleteSession, getMe, listSessionProfiles } from './session';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('session api', () => {
  it('listSessionProfiles GETs /session/profiles and unwraps the envelope', async () => {
    let seenUrl = '';
    let seenMethod = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      seenUrl = String(input);
      seenMethod = init?.method ?? '';
      return new Response(JSON.stringify({ profiles: [{ id: 'alice', name: 'Alice (admin)' }] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const got = await listSessionProfiles();

    expect(seenUrl).toBe('/api/session/profiles');
    expect(seenMethod).toBe('GET');
    expect(got).toEqual([{ id: 'alice', name: 'Alice (admin)' }]);
  });

  it('listSessionProfiles tolerates a null envelope (demo off answers 404, not empty)', async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ profiles: null }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);

    await expect(listSessionProfiles()).resolves.toEqual([]);
  });

  // The contract difference that would silently break the picker: every
  // other honryu mutation is form-encoded, but createSession parses JSON.
  it('createSession POSTs a JSON body to /session', async () => {
    let seenUrl = '';
    let seenMethod = '';
    let seenContentType: string | null = null;
    let seenBody = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      seenUrl = String(input);
      seenMethod = init?.method ?? '';
      seenContentType = new Headers(init?.headers).get('Content-Type');
      seenBody = String(init?.body);
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal('fetch', fetchMock);

    await createSession('bob');

    expect(seenUrl).toBe('/api/session');
    expect(seenMethod).toBe('POST');
    expect(seenContentType).toBe('application/json');
    expect(JSON.parse(seenBody)).toEqual({ profile: 'bob' });
  });

  it('deleteSession DELETEs /session', async () => {
    let seenUrl = '';
    let seenMethod = '';
    const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      seenUrl = String(input);
      seenMethod = init?.method ?? '';
      return new Response(null, { status: 204 });
    });
    vi.stubGlobal('fetch', fetchMock);

    await deleteSession();

    expect(seenUrl).toBe('/api/session');
    expect(seenMethod).toBe('DELETE');
  });

  it('getMe GETs /me and returns the wire shape verbatim', async () => {
    const me = {
      subject: 'demo:bob',
      name: 'Bob (tenant editor)',
      email: '',
      global_roles: [],
      tenants: { '1': ['tenant_editor'] },
      permissions: { execution: ['create', 'delete', 'list', 'read', 'update'] },
      demo: true,
    };
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify(me), { status: 200, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);

    const got = await getMe();

    expect(got).toEqual(me);
  });

  it('getMe surfaces 401 as a typed ApiError the hook branches on', async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ message: 'unauthenticated' }), { status: 401, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);

    const err = await getMe().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(401);
  });
});

describe('can', () => {
  // The four personas' maps, exactly as authapp.Service.Permissions emits
  // them from DefaultCatalog -- the hook and nav filter both sit on this.
  const admin = { '*': ['*'] };
  const viewer = {
    project: ['list', 'read'],
    execution: ['list', 'read'],
    scenario: ['list', 'read'],
    run: ['list', 'read'],
    schedule: ['list', 'read'],
    report: ['list', 'read'],
  };
  const campaignManager = {
    campaign: ['admin', 'create', 'delete', 'list', 'read', 'update'],
    project: ['list', 'read'],
    execution: ['list', 'read'],
    schedule: ['list', 'read'],
    report: ['list', 'read'],
  };

  it('grants anything to the admin wildcard map', () => {
    expect(can(admin, 'system', 'admin')).toBe(true);
    expect(can(admin, 'run', 'create')).toBe(true);
  });

  it('grants only held actions on held resources', () => {
    expect(can(viewer, 'execution', 'read')).toBe(true);
    expect(can(viewer, 'execution', 'delete')).toBe(false); // AC14's core: no delete
    expect(can(viewer, 'run', 'update')).toBe(false); // no stop
    expect(can(viewer, 'campaign', 'read')).toBe(false); // viewer has no campaign surface
    expect(can(viewer, 'system', 'admin')).toBe(false); // no clusters
  });

  it('grants the campaign manager read-only oversight', () => {
    expect(can(campaignManager, 'campaign', 'create')).toBe(true);
    expect(can(campaignManager, 'schedule', 'list')).toBe(true);
    expect(can(campaignManager, 'run', 'create')).toBe(false); // AC10: sees plan, cannot change it
  });

  it('fails closed on absent maps, resources, and per-resource wildcards', () => {
    expect(can(undefined, 'execution', 'read')).toBe(false);
    expect(can(null, 'execution', 'read')).toBe(false);
    expect(can({}, 'execution', 'read')).toBe(false);
    expect(can({ execution: ['*'] }, 'execution', 'delete')).toBe(true);
    expect(can({ execution: ['*'] }, 'scenario', 'read')).toBe(false);
  });
});
