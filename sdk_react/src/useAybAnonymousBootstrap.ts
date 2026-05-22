import { useEffect, useState } from "react";
import { useAuth } from "./useAuth";

interface UseAybAnonymousBootstrapOptions {
  enabled: boolean;
}

export function useAybAnonymousBootstrap({ enabled }: UseAybAnonymousBootstrapOptions) {
  const { token, signInAnonymously } = useAuth();
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
