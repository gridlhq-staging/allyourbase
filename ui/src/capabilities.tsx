import { createContext, useContext, type ReactNode } from "react";
import type {
  AdminCapabilityName,
  AdminCapabilityState,
} from "./api_capabilities";

const OPEN_CAPABILITY_STATE: AdminCapabilityState = { kind: "unknown" };

interface CapabilityContextValue {
  state: AdminCapabilityState;
  canUse: (capability: AdminCapabilityName) => boolean;
}

const CapabilityContext = createContext<CapabilityContextValue>({
  state: OPEN_CAPABILITY_STATE,
  canUse: () => true,
});

function canUseCapability(
  state: AdminCapabilityState,
  capability: AdminCapabilityName,
): boolean {
  if (state.kind === "unknown") {
    return true;
  }
  return state.capabilities[capability];
}

export function CapabilityProvider({
  state,
  children,
}: {
  state: AdminCapabilityState;
  children: ReactNode;
}) {
  const value: CapabilityContextValue = {
    state,
    canUse: (capability) => canUseCapability(state, capability),
  };

  return (
    <CapabilityContext.Provider value={value}>
      {children}
    </CapabilityContext.Provider>
  );
}

export function useCapability(): CapabilityContextValue {
  return useContext(CapabilityContext);
}
