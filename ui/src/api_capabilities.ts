import { fetchAdmin } from "./api_client";

export const ADMIN_CAPABILITY_NAMES = [
  "auth",
  "auth_anonymous",
  "auth_email_mfa",
  "auth_magic_link",
  "auth_oauth_provider",
  "auth_sms",
  "auth_totp",
  "auth_webauthn",
  "billing",
  "edge_functions",
  "jobs",
  "push",
  "status",
  "storage",
  "support",
] as const;

export type AdminCapabilityName = (typeof ADMIN_CAPABILITY_NAMES)[number];
export type AdminCapabilities = Record<AdminCapabilityName, boolean>;

export type AdminCapabilityState =
  | { kind: "known"; capabilities: AdminCapabilities }
  | { kind: "unknown" };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseAdminCapabilities(value: unknown): AdminCapabilityState {
  if (!isRecord(value)) {
    return { kind: "unknown" };
  }

  const capabilities = {} as AdminCapabilities;
  for (const name of ADMIN_CAPABILITY_NAMES) {
    const capability = value[name];
    if (typeof capability !== "boolean") {
      return { kind: "unknown" };
    }
    capabilities[name] = capability;
  }

  return { kind: "known", capabilities };
}

export async function getAdminCapabilities(): Promise<AdminCapabilityState> {
  try {
    const response = await fetchAdmin("/api/admin/capabilities");
    if (!response.ok) {
      return { kind: "unknown" };
    }
    return parseAdminCapabilities(await response.json());
  } catch {
    return { kind: "unknown" };
  }
}
