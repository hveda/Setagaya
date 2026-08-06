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
});
