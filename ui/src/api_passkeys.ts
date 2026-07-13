import type { AuthTokens } from "./types";
import { requestAuth, requestAuthNoBody, setAuthToken } from "./api_client";
import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from "./webauthn";

export interface PasskeyChallenge {
  challenge_id: string;
  options: PublicKeyCredentialRequestOptionsJSON;
}

export interface PasskeyCredentialMetadata {
  credentialId: string;
  displayName: string;
  transports: string[];
  createdAt: string;
  lastUsedAt?: string;
}

const PASSKEY_CREDENTIALS_PATH = "/api/auth/mfa/webauthn/credentials";

async function requestAndStoreAuthToken(
  path: string,
  init?: RequestInit,
): Promise<AuthTokens> {
  const tokens = await requestAuth<AuthTokens>(path, init);
  setAuthToken(tokens.token);
  return tokens;
}

function requireRecord(value: unknown, message: string): Record<string, unknown> {
  if (typeof value !== "object" || value === null || Array.isArray(value)) {
    throw new Error(message);
  }
  return value as Record<string, unknown>;
}

function requireStringField(
  source: Record<string, unknown>,
  field: string,
  message: string,
): string {
  const value = source[field];
  if (typeof value !== "string") {
    throw new Error(message);
  }
  return value;
}

function normalizeTransports(value: unknown): string[] {
  if (!Array.isArray(value) || value.some((transport) => typeof transport !== "string")) {
    throw new Error("Invalid passkey credential metadata");
  }
  return [...value];
}

function normalizePasskeyCredentialMetadata(value: unknown): PasskeyCredentialMetadata {
  const source = requireRecord(value, "Invalid passkey credential metadata");
  const lastUsedAt = source.last_used_at;
  if (lastUsedAt !== undefined && lastUsedAt !== null && typeof lastUsedAt !== "string") {
    throw new Error("Invalid passkey credential metadata");
  }

  const metadata: PasskeyCredentialMetadata = {
    credentialId: requireStringField(source, "credential_id", "Invalid passkey credential metadata"),
    displayName: requireStringField(source, "display_name", "Invalid passkey credential metadata"),
    transports: normalizeTransports(source.transports),
    createdAt: requireStringField(source, "created_at", "Invalid passkey credential metadata"),
  };
  if (typeof lastUsedAt === "string") {
    metadata.lastUsedAt = lastUsedAt;
  }
  return metadata;
}

function normalizePasskeyCredentialsEnvelope(value: unknown): PasskeyCredentialMetadata[] {
  const source = requireRecord(value, "Invalid passkey credentials response");
  if (!Array.isArray(source.credentials)) {
    throw new Error("Invalid passkey credentials response");
  }
  return source.credentials.map((credential) => normalizePasskeyCredentialMetadata(credential));
}

export async function beginPasskeyEnroll(): Promise<PublicKeyCredentialCreationOptionsJSON> {
  return requestAuth("/api/auth/mfa/webauthn/enroll", { method: "POST" });
}

export async function confirmPasskeyEnroll(
  displayName: string,
  attestationResponse: Record<string, unknown>,
): Promise<{ message?: string; token?: string }> {
  const response = await requestAuth<{ message?: string; token?: string }>("/api/auth/mfa/webauthn/enroll/confirm", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      display_name: displayName,
      attestation_response: attestationResponse,
    }),
  });
  if (typeof response.token === "string" && response.token.length > 0) {
    setAuthToken(response.token);
  }
  return response;
}

export async function beginPasskeyChallenge(): Promise<PasskeyChallenge> {
  return requestAuth("/api/auth/mfa/webauthn/challenge", { method: "POST" });
}

export async function verifyPasskeyChallenge(
  challengeId: string,
  assertionResponse: Record<string, unknown>,
): Promise<AuthTokens> {
  return requestAndStoreAuthToken("/api/auth/mfa/webauthn/verify", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      challenge_id: challengeId,
      assertion_response: assertionResponse,
    }),
  });
}

export async function listPasskeys(): Promise<PasskeyCredentialMetadata[]> {
  const response = await requestAuth<unknown>(PASSKEY_CREDENTIALS_PATH, { method: "GET" });
  return normalizePasskeyCredentialsEnvelope(response);
}

export async function renamePasskey(
  credentialId: string,
  displayName: string,
): Promise<PasskeyCredentialMetadata> {
  return normalizePasskeyCredentialMetadata(
    await requestAuth<unknown>(`${PASSKEY_CREDENTIALS_PATH}/${encodeURIComponent(credentialId)}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ display_name: displayName }),
    }),
  );
}

export async function deletePasskey(credentialId: string): Promise<void> {
  await requestAuthNoBody(`${PASSKEY_CREDENTIALS_PATH}/${encodeURIComponent(credentialId)}`, { method: "DELETE" });
}
