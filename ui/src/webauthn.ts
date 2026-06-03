/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/may31_pm_10_webauthn_passkeys_ui_e2e/allyourbase_dev/ui/src/webauthn.ts.
 */
interface PublicKeyCredentialDescriptorJSON {
  id: string;
  type: PublicKeyCredentialType;
  transports?: AuthenticatorTransport[];
}

interface PublicKeyCredentialUserEntityJSON {
  id: string;
  name: string;
  displayName: string;
}

export interface PublicKeyCredentialCreationOptionsJSON {
  challenge: string;
  rp: PublicKeyCredentialRpEntity;
  user: PublicKeyCredentialUserEntityJSON;
  pubKeyCredParams: PublicKeyCredentialParameters[];
  timeout?: number;
  attestation?: AttestationConveyancePreference;
  excludeCredentials?: PublicKeyCredentialDescriptorJSON[];
  authenticatorSelection?: AuthenticatorSelectionCriteria;
  extensions?: AuthenticationExtensionsClientInputs;
}

export interface PublicKeyCredentialRequestOptionsJSON {
  challenge: string;
  timeout?: number;
  rpId?: string;
  allowCredentials?: PublicKeyCredentialDescriptorJSON[];
  userVerification?: UserVerificationRequirement;
  extensions?: AuthenticationExtensionsClientInputs;
}

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

function assertWebAuthnAvailable(): void {
  if (typeof window === "undefined" || typeof navigator === "undefined" || !navigator.credentials) {
    throw new Error("This browser does not support WebAuthn credentials");
  }
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

function ensurePublicKeyCredential(credential: Credential | null): PublicKeyCredential {
  if (!credential || !(credential instanceof PublicKeyCredential)) {
    throw new Error("The browser did not return a WebAuthn credential");
  }
  return credential;
}

type AuthTokenPayload = {
  aal?: unknown;
  is_anonymous?: unknown;
};

function decodeAuthTokenPayload(token: string | null): AuthTokenPayload | null {
  if (!token) {
    return null;
  }

  const parts = token.split(".");
  if (parts.length < 2) {
    return null;
  }

  try {
    const normalized = parts[1].replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
    return JSON.parse(atob(padded)) as AuthTokenPayload;
  } catch {
    return null;
  }
}

export function serializePasskeyAttestation(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response;
  if (!(response instanceof AuthenticatorAttestationResponse)) {
    throw new Error("The browser returned an unexpected passkey attestation response");
  }

  return {
    id: credential.id,
    rawId: encodeBase64URL(credential.rawId),
    type: credential.type,
    response: {
      clientDataJSON: encodeBase64URL(response.clientDataJSON),
      attestationObject: encodeBase64URL(response.attestationObject),
      transports: typeof response.getTransports === "function" ? response.getTransports() : [],
    },
    clientExtensionResults: credential.getClientExtensionResults(),
  };
}

export function serializePasskeyAssertion(credential: PublicKeyCredential): Record<string, unknown> {
  const response = credential.response;
  if (!(response instanceof AuthenticatorAssertionResponse)) {
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

export async function createPasskeyAttestation(
  options: PublicKeyCredentialCreationOptionsJSON,
): Promise<Record<string, unknown>> {
  assertWebAuthnAvailable();
  const credential = ensurePublicKeyCredential(
    await navigator.credentials.create({ publicKey: toCreationOptions(options) }),
  );
  return serializePasskeyAttestation(credential);
}

export async function createPasskeyAssertion(
  options: PublicKeyCredentialRequestOptionsJSON,
): Promise<Record<string, unknown>> {
  assertWebAuthnAvailable();
  const credential = ensurePublicKeyCredential(
    await navigator.credentials.get({ publicKey: toRequestOptions(options) }),
  );
  return serializePasskeyAssertion(credential);
}

export function readAALFromAuthToken(token: string | null): string | null {
  const payload = decodeAuthTokenPayload(token);
  return typeof payload?.aal === "string" ? payload.aal : null;
}

export function readIsAnonymousFromAuthToken(token: string | null): boolean | null {
  const payload = decodeAuthTokenPayload(token);
  return typeof payload?.is_anonymous === "boolean" ? payload.is_anonymous : null;
}
