import { useCallback, useEffect, useState } from "react";
import { useAYBClient } from "./provider";

interface UseAybAnonymousBootstrapOptions {
  enabled: boolean;
  token?: string | null;
  signInAnonymously?: () => Promise<void>;
}

export function useAybAnonymousBootstrap({
  enabled,
  token: tokenOverride,
  signInAnonymously: signInAnonymouslyOverride,
}: UseAybAnonymousBootstrapOptions) {
  const client = useAYBClient();
  const token = tokenOverride ?? client.token;
  const signInAnonymouslyFromClient = useCallback(async () => {
    await client.auth.signInAnonymously();
  }, [client]);
  const signInAnonymously = signInAnonymouslyOverride ?? signInAnonymouslyFromClient;
  const [bootstrapping, setBootstrapping] = useState(false);

  useEffect(() => {
    if (!enabled || token) {
      setBootstrapping(false);
      return;
    }

    let mounted = true;
    setBootstrapping(true);
    const bootstrapAnonymous = async () => {
      try {
        await signInAnonymously();
      } catch {
        // useAuth owns auth error state, so this hook only prevents unhandled rejections.
      } finally {
        if (mounted) {
          setBootstrapping(false);
        }
      }
    };
    void bootstrapAnonymous();

    return () => {
      mounted = false;
    };
  }, [enabled, token, signInAnonymously]);

  return { bootstrapping };
}
