// useSession: the SPA's single source of identity (phase 20 spec, Approach
// F). One provider fetches /api/me on mount and every consumer -- the
// picker, the nav, every action button -- reads the same context, so the UI
// cannot drift from what the server just decided.
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { ApiError } from '../api/client';
import { can as canOn, deleteSession, getMe, listSessionProfiles } from '../api/session';
import type { SessionInfo, SessionProfile } from '../api/session';

/** What useSession() exposes; `session` is null whenever unauthenticated. */
export interface SessionContextValue {
  /** True while the mount-time /api/me is in flight. */
  loading: boolean;
  /** The authenticated identity, or null (401 / not yet resolved). */
  session: SessionInfo | null;
  /** Why /api/me failed when it failed with something other than 401. */
  error: string | null;
  /** The picker's persona list, loaded only when unauthenticated. */
  profiles: SessionProfile[];
  /** Why the profile list is unavailable (demo off, API down). */
  profilesError: string | null;
  /** Permission check against the current session; false when unauthenticated. */
  can: (resource: string, action: string) => boolean;
  /** Logs out (DELETE /api/session) and re-resolves identity. */
  logout: () => Promise<void>;
  /** Re-fetches /api/me (and the profile list when that says 401). */
  refresh: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

/**
 * Resolves one /api/me attempt into context state. Exported pure so the
 * 401-vs-error-vs-session branching is testable without mounting React.
 * A 401 is not an error: it is the unauthenticated state the picker lives
 * in. Only transport/server failures set `error`.
 */
export function resolveMeOutcome(status: number, me: SessionInfo | null, message: string): {
  session: SessionInfo | null;
  error: string | null;
} {
  if (status === 0) {
    return { session: null, error: message || 'network error' };
  }
  if (status === 401) {
    return { session: null, error: null };
  }
  if (me !== null) {
    return { session: me, error: null };
  }
  return { session: null, error: message || `request failed with status ${status}` };
}

/**
 * Mounts once per app load. /api/me is fetched on mount and again after
 * logout; the persona list is fetched only once the caller is known
 * unauthenticated -- the picker is the only consumer, and in non-demo
 * modes the endpoint 404s, so there is no reason to ask before it matters.
 */
export function SessionProvider({ children }: { children: ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [session, setSession] = useState<SessionInfo | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [profiles, setProfiles] = useState<SessionProfile[]>([]);
  const [profilesError, setProfilesError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    let outcome: { session: SessionInfo | null; error: string | null } | null = null;
    try {
      const me = await getMe();
      outcome = resolveMeOutcome(200, me, '');
    } catch (err) {
      if (err instanceof ApiError) {
        outcome = resolveMeOutcome(err.status, null, err.message);
      } else {
        outcome = resolveMeOutcome(0, null, err instanceof Error ? err.message : 'network error');
      }
    }
    setSession(outcome.session);
    setError(outcome.error);
    if (outcome.session === null && outcome.error === null) {
      // Unauthenticated: this is the state the picker needs its list for.
      // A failure here is reported separately -- demo off (404) reads as
      // "no personas", not as a broken app.
      try {
        setProfiles(await listSessionProfiles());
        setProfilesError(null);
      } catch (err) {
        setProfiles([]);
        setProfilesError(err instanceof ApiError ? err.message : 'Failed to load profiles.');
      }
    } else {
      setProfiles([]);
      setProfilesError(null);
    }
    setLoading(false);
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const logout = useCallback(async () => {
    try {
      await deleteSession();
    } finally {
      // Even when the DELETE fails (API down, cookie already gone), the
      // honest next move is the same: re-ask the server who we are now.
      await refresh();
    }
  }, [refresh]);

  const can = useCallback(
    (resource: string, action: string) => canOn(session?.permissions, resource, action),
    [session]
  );

  const value = useMemo(
    () => ({ loading, session, error, profiles, profilesError, can, logout, refresh }),
    [loading, session, error, profiles, profilesError, can, logout, refresh]
  );

  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

/** Reads the app's identity. Throws when used outside SessionProvider. */
export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (ctx === null) {
    throw new Error('useSession must be used within SessionProvider');
  }
  return ctx;
}
