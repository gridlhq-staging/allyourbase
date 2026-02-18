import { useEffect } from "react";
import { ayb } from "../lib/ayb";

export interface RealtimeEvent {
  action: "create" | "update" | "delete";
  table: string;
  record: Record<string, unknown>;
}

/**
 * Subscribe to realtime changes on the given tables.
 * Calls `callback` for every create/update/delete event.
 */
export function useRealtime(
  tables: string[],
  callback: (event: RealtimeEvent) => void,
) {
  useEffect(() => {
    if (tables.length === 0) return;
    const unsub = ayb.realtime.subscribe(tables, callback as never);
    return unsub;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tables.join(",")]);
}
