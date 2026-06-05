import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import type { AYBClientLike } from "./types";

const AYBContext = createContext<AYBClientLike | null>(null);

export interface AYBProviderProps {
  client: AYBClientLike;
  children?: unknown;
}

export function AYBProvider({ client, children }: AYBProviderProps) {
  return <AYBContext.Provider value={client}>{children as ReactNode}</AYBContext.Provider>;
}

export function useAYBClient(): AYBClientLike {
  const client = useContext(AYBContext);
  if (!client) {
    throw new Error("useAYBClient must be used within AYBProvider");
  }
  return client;
}
