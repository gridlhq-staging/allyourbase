import type { APIRequestContext } from "@playwright/test";
import { readFileSync } from "fs";
import { join } from "path";
import { homedir } from "os";

export type AdminBootstrapCredential = {
  source: "env-password" | "saved-admin-auth";
  value: string;
};

const MISSING_ADMIN_BOOTSTRAP_CREDENTIAL_MESSAGE =
  "No admin password found. Either set AYB_ADMIN_PASSWORD or ensure `ayb start` is running (writes ~/.ayb/admin-token).";

class MissingAdminBootstrapCredentialError extends Error {
  constructor() {
    super(MISSING_ADMIN_BOOTSTRAP_CREDENTIAL_MESSAGE);
    this.name = "MissingAdminBootstrapCredentialError";
  }
}

function isMissingSavedAdminAuthError(error: unknown): boolean {
  return (
    error instanceof Error &&
    (error as Error & { code?: string }).code === "ENOENT"
  );
}

function describeError(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return String(error);
}

function readSavedAdminAuth(): string {
  const tokenPath = join(homedir(), ".ayb", "admin-token");
  const savedAdminAuth = readFileSync(tokenPath, "utf-8").trim();
  if (savedAdminAuth.length === 0) {
    throw new Error("Saved admin auth file is empty");
  }
  return savedAdminAuth;
}

export function resolveAdminBootstrapCredential(): AdminBootstrapCredential {
  if (process.env.AYB_ADMIN_PASSWORD) {
    return { source: "env-password", value: process.env.AYB_ADMIN_PASSWORD };
  }

  try {
    return {
      source: "saved-admin-auth",
      value: readSavedAdminAuth(),
    };
  } catch (error) {
    if (isMissingSavedAdminAuthError(error)) {
      throw new MissingAdminBootstrapCredentialError();
    }

    throw new Error(
      `Failed to read saved admin auth from ~/.ayb/admin-token: ${describeError(error)}`,
    );
  }
}

// The standalone login smoke needs an actual password to exercise the form.
// If the saved file already contains a bearer token, skip that positive-path
// form login instead of submitting the token as if it were a password.
export async function resolveAdminPasswordForBrowserLogin(
  request: APIRequestContext,
): Promise<string | null> {
  let credential: AdminBootstrapCredential;
  try {
    credential = resolveAdminBootstrapCredential();
  } catch (error) {
    if (!(error instanceof MissingAdminBootstrapCredentialError)) {
      throw error;
    }
    return null;
  }

  if (credential.source === "env-password") {
    return credential.value;
  }

  const loginRes = await request.post("/api/admin/auth", {
    data: { password: credential.value },
  });
  if (loginRes.ok()) {
    return credential.value;
  }
  if (loginRes.status() === 401) {
    return null;
  }

  throw new Error(
    `Admin password probe failed with status ${loginRes.status()} while checking saved admin auth`,
  );
}
