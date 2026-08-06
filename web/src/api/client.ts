// Base fetch wrapper for the honryu API: attaches the auth header, and
// surfaces the backend's error envelope ({"message": "..."}, see
// internal/adapters/httpapi/response.go's writeError) as a typed ApiError
// instead of a generic fetch failure.

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export interface ApiClientOptions {
  /** Defaults to "/api", proxied by Vite in dev and same-origin in prod (cmd/api serves both). */
  baseUrl?: string;
  /** Defaults to reading "honryu_token" from localStorage; returns null when unauthenticated (no-auth mode). */
  getToken?: () => string | null;
}

export class ApiClient {
  /** Exposed read-only so callers needing a raw URL (e.g. EventSource, which can't use fetch) can build one. */
  readonly baseUrl: string;
  private readonly getToken: () => string | null;

  constructor(options: ApiClientOptions = {}) {
    this.baseUrl = options.baseUrl ?? '/api';
    this.getToken = options.getToken ?? (() => localStorage.getItem('honryu_token'));
  }

  async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set('Accept', 'application/json');
    const token = this.getToken();
    if (token) {
      headers.set('Authorization', `Bearer ${token}`);
    }

    const res = await fetch(`${this.baseUrl}${path}`, { ...init, headers });
    if (!res.ok) {
      throw new ApiError(res.status, await extractErrorMessage(res));
    }
    if (res.status === 204) {
      return undefined as T;
    }
    return (await res.json()) as T;
  }

  get<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: 'GET' });
  }
}

async function extractErrorMessage(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { message?: unknown };
    if (typeof body.message === 'string' && body.message !== '') {
      return body.message;
    }
  } catch {
    // Non-JSON or empty error body; fall through to statusText.
  }
  return res.statusText || `request failed with status ${res.status}`;
}

/** The client every page uses; a fresh ApiClient() with custom options is only needed in tests. */
export const apiClient = new ApiClient();
