/**
 * @module React hook for accessing authentication state, user data, and login/logout functionality with automatic session synchronization.
 */
import { useCallback, useEffect, useState } from "react";
import { useAYBClient } from "./provider";
import type { OAuthOptions, OAuthProvider, UseAuthResult, UserLike } from "./types";

function isUnauthorizedError(err: unknown): boolean {
  if (!err || typeof err !== "object") {
    return false;
  }
  const status = (err as { status?: unknown }).status;
  return status === 401 || status === 403;
}

function authUserFromResponse(response: unknown): UserLike | null {
  if (!response || typeof response !== "object") {
    return null;
  }
  const user = (response as { user?: unknown }).user;
  if (!user || typeof user !== "object") {
    return null;
  }
  const id = (user as { id?: unknown }).id;
  return typeof id === "string" ? (user as UserLike) : null;
}

/**
 * Manages authentication state and automatically syncs with the client's auth provider. Loads the current user on mount and resubscribes to auth state changes, handling token updates and session management. Returns current user data, tokens, loading/error states, and authentication methods.
 */
export function useAuth(): UseAuthResult {
  const client = useAYBClient();
  const [loading, setLoading] = useState<boolean>(Boolean(client.token));
  const [user, setUser] = useState<UserLike | null>(null);
  const [error, setError] = useState<Error | null>(null);
  const [token, setToken] = useState<string | null>(client.token);
  const [refreshToken, setRefreshToken] = useState<string | null>(client.refreshToken);

  const loadMe = useCallback(async () => {
    if (!client.token) {
      setUser(null);
      setLoading(false);
      return;
    }

    setLoading(true);
    let unauthorizedSession = false;
    try {
      const me = await client.auth.me();
      setUser(me);
      setError(null);
    } catch (err) {
      if (isUnauthorizedError(err)) {
        unauthorizedSession = true;
        client.clearTokens?.();
      }
      setUser(null);
      setError(err as Error);
    } finally {
      setLoading(false);
      setToken(unauthorizedSession ? null : client.token);
      setRefreshToken(unauthorizedSession ? null : client.refreshToken);
    }
  }, [client]);

  const syncAuthResponse = useCallback(
    async (response: unknown) => {
      const responseUser = authUserFromResponse(response);
      if (!responseUser) {
        await loadMe();
        return;
      }
      setUser(responseUser);
      setError(null);
      setLoading(false);
      setToken(client.token);
      setRefreshToken(client.refreshToken);
    },
    [client, loadMe],
  );

  useEffect(() => {
    let mounted = true;

    const run = async () => {
      try {
        if (client.waitForSessionRestore) {
          await client.waitForSessionRestore();
        }
        if (mounted) {
          await loadMe();
        }
      } catch {
        // loadMe sets local error state.
      }
    };

    void run();

    const unsubscribe = client.onAuthStateChange((event, session) => {
      if (!mounted) return;

      setToken(session?.token ?? client.token);
      setRefreshToken(session?.refreshToken ?? client.refreshToken);

      if (event === "SIGNED_OUT") {
        setUser(null);
        setError(null);
        setLoading(false);
        return;
      }

      void loadMe();
    });

    return () => {
      mounted = false;
      unsubscribe();
    };
  }, [client, loadMe]);

  const login = useCallback(
    async (email: string, password: string) => {
      const response = await client.auth.login(email, password);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const register = useCallback(
    async (email: string, password: string) => {
      const response = await client.auth.register(email, password);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const signInAnonymously = useCallback(async () => {
    const response = await client.auth.signInAnonymously();
    await syncAuthResponse(response);
  }, [client, syncAuthResponse]);

  const signInWithPasskey = useCallback(
    async (email: string) => {
      const signInWithPasskeyRequest = client.auth.signInWithPasskey;
      if (!signInWithPasskeyRequest) {
        throw new Error("Passkey sign-in is not available for this client");
      }
      const response = await signInWithPasskeyRequest.call(client.auth, email);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const requestMagicLink = useCallback(
    async (email: string) => {
      await client.auth.requestMagicLink(email);
    },
    [client],
  );

  const confirmMagicLink = useCallback(
    async (token: string) => {
      const response = await client.auth.confirmMagicLink(token);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const linkEmail = useCallback(
    async (email: string, password: string) => {
      const response = await client.auth.linkEmail(email, password);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const signInWithOAuth = useCallback(
    async (provider: OAuthProvider, options?: OAuthOptions) => {
      const response = await client.auth.signInWithOAuth(provider, options);
      await syncAuthResponse(response);
    },
    [client, syncAuthResponse],
  );

  const logout = useCallback(async () => {
    await client.auth.logout();
    setUser(null);
    setError(null);
    setToken(client.token);
    setRefreshToken(client.refreshToken);
  }, [client]);

  const refresh = useCallback(async () => {
    const response = await client.auth.refresh();
    await syncAuthResponse(response);
  }, [client, syncAuthResponse]);

  return {
    loading,
    user,
    error,
    token,
    refreshToken,
    login,
    register,
    signInAnonymously,
    signInWithPasskey,
    requestMagicLink,
    confirmMagicLink,
    linkEmail,
    signInWithOAuth,
    logout,
    refresh,
  };
}
