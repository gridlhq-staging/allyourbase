import { useEffect } from "react";
import type { RealtimeEvent } from "@allyourbase/js";
import { ayb } from "../lib/ayb";

export function useRealtime(
  tables: string[],
  callback: (event: RealtimeEvent) => void,
) {
  useEffect(() => {
    if (tables.length === 0) return;
    let disposed = false;
    let unsubscribe: (() => void) | null = null;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;

    const connect = () => {
      void ayb.realtime
        .subscribeWS(tables, callback)
        .then((unsub) => {
          if (disposed) {
            unsub();
            return;
          }
          unsubscribe = unsub;
        })
        .catch((err) => {
          if (disposed) return;
          console.error("Failed to establish realtime WS subscription:", err);
          retryTimer = setTimeout(connect, 500);
        });
    };

    connect();

    return () => {
      disposed = true;
      if (retryTimer) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
      if (unsubscribe) {
        unsubscribe();
        }
    };
  }, [tables.join(",")]); // eslint-disable-line react-hooks/exhaustive-deps
}
