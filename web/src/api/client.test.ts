import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiClient, ApiError } from './client';

describe('ApiClient', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('GETs against baseUrl and returns the decoded JSON body', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input)).toBe('/mock-api/scenarios/1');
      return new Response(JSON.stringify({ id: 1, name: 'checkout' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    const got = await client.get<{ id: number; name: string }>('/scenarios/1');

    expect(got).toEqual({ id: 1, name: 'checkout' });
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it('attaches a bearer token when one is available', async () => {
    let seenAuth: string | null = null;
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      seenAuth = new Headers(init?.headers).get('Authorization');
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => 'tok123' });
    await client.get('/whoami');

    expect(seenAuth).toBe('Bearer tok123');
  });

  it('omits the Authorization header when there is no token', async () => {
    let seenAuth: string | null = 'unset';
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      seenAuth = new Headers(init?.headers).get('Authorization');
      return new Response('{}', { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    await client.get('/whoami');

    expect(seenAuth).toBeNull();
  });

  it('surfaces the backend error envelope as a typed ApiError', async () => {
    const fetchMock = vi.fn(async () => {
      return new Response(JSON.stringify({ message: 'execution not found' }), {
        status: 404,
        headers: { 'Content-Type': 'application/json' },
      });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    await expect(client.get('/executions/999')).rejects.toMatchObject({
      name: 'ApiError',
      status: 404,
      message: 'execution not found',
    });
  });

  it('falls back to statusText when the error body is not the expected shape', async () => {
    const fetchMock = vi.fn(async () => new Response('not json', { status: 500, statusText: 'Internal Server Error' }));
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    let caught: unknown;
    try {
      await client.get('/boom');
    } catch (err) {
      caught = err;
    }
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).status).toBe(500);
    expect((caught as ApiError).message).toBe('Internal Server Error');
  });

  it('returns undefined for a 204 No Content response', async () => {
    const fetchMock = vi.fn(async () => new Response(null, { status: 204 }));
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    const got = await client.get('/nothing');
    expect(got).toBeUndefined();
  });

  it('POSTs a form-urlencoded body', async () => {
    let seenMethod = '';
    let seenContentType: string | null = null;
    let seenBody = '';
    const fetchMock = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      seenMethod = init?.method ?? '';
      seenContentType = new Headers(init?.headers).get('Content-Type');
      seenBody = String(init?.body);
      return new Response(JSON.stringify({ ok: true }), { status: 201, headers: { 'Content-Type': 'application/json' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    const form = new URLSearchParams();
    form.set('name', 'checkout');
    const got = await client.post<{ ok: boolean }>('/scenarios', form);

    expect(seenMethod).toBe('POST');
    expect(seenContentType).toBe('application/x-www-form-urlencoded');
    expect(seenBody).toBe('name=checkout');
    expect(got).toEqual({ ok: true });
  });

  it('text() GETs and returns the raw text/plain body', async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      expect(String(input)).toBe('/mock-api/runs/1/scenarios/2/shards/0/config');
      return new Response('concurrency: 10', { status: 200, headers: { 'Content-Type': 'text/plain; charset=utf-8' } });
    });
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    const got = await client.text('/runs/1/scenarios/2/shards/0/config');

    expect(got).toBe('concurrency: 10');
  });

  it('text() surfaces the error envelope like JSON requests do', async () => {
    const fetchMock = vi.fn(
      async () => new Response(JSON.stringify({ message: 'shard not found' }), { status: 404, headers: { 'Content-Type': 'application/json' } })
    );
    vi.stubGlobal('fetch', fetchMock);

    const client = new ApiClient({ baseUrl: '/mock-api', getToken: () => null });
    await expect(client.text('/runs/1/scenarios/2/shards/9/config')).rejects.toMatchObject({
      name: 'ApiError',
      status: 404,
      message: 'shard not found',
    });
  });
});

describe('putRaw', () => {
  it('PUTs the raw body with the given content type and bearer token', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
    vi.stubGlobal('fetch', fetchMock);
    const client = new ApiClient({ baseUrl: 'http://x', getToken: () => 'tok' });
    await client.putRaw('/scenarios/1/requests', 'text/yaml', 'a: 1\n');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe('http://x/scenarios/1/requests');
    expect(init.method).toBe('PUT');
    expect(init.headers.get('Content-Type')).toBe('text/yaml');
    expect(init.headers.get('Authorization')).toBe('Bearer tok');
    expect(init.body).toBe('a: 1\n');
  });

  it('throws ApiError with the server message on failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { message: 'invalid fragment' } }), { status: 422 }),
      ),
    );
    const client = new ApiClient({ baseUrl: 'http://x', getToken: () => null });
    const err = await client.putRaw('/scenarios/1/requests', 'text/yaml', 'a: 1\n').catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(422);
  });
});
