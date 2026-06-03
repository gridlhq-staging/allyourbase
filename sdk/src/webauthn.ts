/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun01_pm_5_js_sdk_search_passkey/allyourbase_dev/sdk/src/webauthn.ts.
 */
import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from "./types";

function decodeBase64URL(value: string): ArrayBuffer {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
  const binary = atob(padded);
  const bytes = Uint8Array.from(binary, (character) => character.charCodeAt(0));
  return bytes.buffer;
}

function encodeBase64URL(buffer: ArrayBuffer): string {
  const binary = String.fromCharCode(...new Uint8Array(buffer));
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

interface WebAuthnConstructors {
  PublicKeyCredential: typeof PublicKeyCredential;
  AuthenticatorAssertionResponse: typeof AuthenticatorAssertionResponse;
  AuthenticatorAttestationResponse: typeof AuthenticatorAttestationResponse;
}

function getWebAuthnConstructors(): WebAuthnConstructors | null {
  if (
    typeof PublicKeyCredential !== "function"
    || typeof AuthenticatorAssertionResponse !== "function"
    || typeof AuthenticatorAttestationResponse !== "function"
  ) {
    return null;
  }
  return {
    PublicKeyCredential,
    AuthenticatorAssertionResponse,
    AuthenticatorAttestationResponse,
  };
}

function assertWebAuthnAvailable(): WebAuthnConstructors {
  const constructors = getWebAuthnConstructors();
  if (typeof navigator === "undefined" || !navigator.credentials || !constructors) {
    throw new Error("This browser does not support WebAuthn credentials");
  }
  return constructors;
}

function toRequestOptions(
  options: PublicKeyCredentialRequestOptionsJSON,
): PublicKeyCredentialRequestOptions {
  return {
    ...options,
    challenge: decodeBase64URL(options.challenge),
    allowCredentials: options.allowCredentials?.map((credential) => ({
      ...credential,
      id: decodeBase64URL(credential.id),
    })),
  };
}

function ensurePublicKeyCredential(
  credential: Credential | null,
  PublicKeyCredentialConstructor: typeof PublicKeyCredential,
): PublicKeyCredential {
  if (!credential || !(credential instanceof PublicKeyCredentialConstructor)) {
    throw new Error("The browser did not return a WebAuthn credential");
  }
  return credential;
}

export function serializePasskeyAssertion(
  credential: PublicKeyCredential,
  AuthenticatorAssertionResponseConstructor: typeof AuthenticatorAssertionResponse,
): Record<string, unknown> {
  const response = credential.response;
  if (!(response instanceof AuthenticatorAssertionResponseConstructor)) {
    throw new Error("The browser returned an unexpected passkey assertion response");
  }

  return {
    id: credential.id,
    rawId: encodeBase64URL(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: encodeBase64URL(response.clientDataJSON),
      authenticatorData: encodeBase64URL(response.authenticatorData),
      signature: encodeBase64URL(response.signature),
      userHandle: response.userHandle ? encodeBase64URL(response.userHandle) : null,
    },
    clientExtensionResults: credential.getClientExtensionResults(),
  };
}

export async function createPasskeyAssertion(
  options: PublicKeyCredentialRequestOptionsJSON,
): Promise<Record<string, unknown>> {
  const constructors = assertWebAuthnAvailable();
  const credential = ensurePublicKeyCredential(
    await navigator.credentials.get({ publicKey: toRequestOptions(options) }),
    constructors.PublicKeyCredential,
  );
  return serializePasskeyAssertion(credential, constructors.AuthenticatorAssertionResponse);
}

function toCreationOptions(
  options: PublicKeyCredentialCreationOptionsJSON,
): PublicKeyCredentialCreationOptions {
  return {
    ...options,
    challenge: decodeBase64URL(options.challenge),
    user: {
      ...options.user,
      id: decodeBase64URL(options.user.id),
    },
    excludeCredentials: options.excludeCredentials?.map((credential) => ({
      ...credential,
      id: decodeBase64URL(credential.id),
    })),
  };
}

/**
 * Serialize a WebAuthn `PublicKeyCredential` returned from
 * `navigator.credentials.create` into the JSON shape the backend
 * `/api/auth/mfa/webauthn/enroll/confirm` endpoint expects:
 * base64url `clientDataJSON` + `attestationObject`, with `rawId`
 * also base64url-encoded.
 */
export function serializePasskeyAttestation(
  credential: PublicKeyCredential,
  AuthenticatorAttestationResponseConstructor: typeof AuthenticatorAttestationResponse,
): Record<string, unknown> {
  const response = credential.response;
  if (!(response instanceof AuthenticatorAttestationResponseConstructor)) {
    throw new Error("The browser returned an unexpected passkey attestation response");
  }

  return {
    id: credential.id,
    rawId: encodeBase64URL(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: encodeBase64URL(response.clientDataJSON),
      attestationObject: encodeBase64URL(response.attestationObject),
    },
    clientExtensionResults: credential.getClientExtensionResults(),
  };
}

export async function createPasskeyAttestation(
  options: PublicKeyCredentialCreationOptionsJSON,
): Promise<Record<string, unknown>> {
  const constructors = assertWebAuthnAvailable();
  const credential = ensurePublicKeyCredential(
    await navigator.credentials.create({ publicKey: toCreationOptions(options) }),
    constructors.PublicKeyCredential,
  );
  return serializePasskeyAttestation(credential, constructors.AuthenticatorAttestationResponse);
}
