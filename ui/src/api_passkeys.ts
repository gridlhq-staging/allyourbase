/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/may31_pm_10_webauthn_passkeys_ui_e2e/allyourbase_dev/ui/src/api_passkeys.ts.
 */
import type { AuthTokens, MFAFactor } from "./types";
import { requestAuth, requestAuthNoBody, setAuthToken } from "./api_client";
import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from "./webauthn";

export interface PasskeyChallenge {
  challenge_id: string;
  options: PublicKeyCredentialRequestOptionsJSON;
}

async function requestAndStoreAuthToken(
  path: string,
  init?: RequestInit,
): Promise<AuthTokens> {
  const tokens = await requestAuth<AuthTokens>(path, init);
  setAuthToken(tokens.token);
  return tokens;
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

export async function listPasskeys(): Promise<MFAFactor[]> {
  const response = await requestAuth<{ factors: MFAFactor[] }>("/api/auth/mfa/factors");
  return response.factors.filter((factor) => factor.method === "webauthn");
}

export async function deletePasskey(): Promise<void> {
  await requestAuthNoBody("/api/auth/mfa/webauthn/", { method: "DELETE" });
}
