/** @module Browser-test fixtures for admin capability probes. */
import type { APIRequestContext } from "@playwright/test";
import { validateResponse } from "./core";

const ADMIN_CAPABILITY_NAMES = [
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

function parseAdminCapabilities(value: unknown): AdminCapabilities {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error("Expected admin capabilities response to be a JSON object");
  }

  const body = value as Record<string, unknown>;
  const capabilities = {} as AdminCapabilities;
  for (const name of ADMIN_CAPABILITY_NAMES) {
    const capability = body[name];
    if (typeof capability !== "boolean") {
      throw new Error(`Expected boolean admin capability ${name}`);
    }
    capabilities[name] = capability;
  }
  return capabilities;
}

export async function getAdminCapabilities(
  request: APIRequestContext,
  token: string,
): Promise<AdminCapabilities> {
  const res = await request.get("/api/admin/capabilities", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "Get admin capabilities");
  return parseAdminCapabilities(await res.json());
}
