import { afterEach, describe, expect, it, vi } from "vitest";
import type {
  PublicKeyCredentialCreationOptionsJSON,
  PublicKeyCredentialRequestOptionsJSON,
} from "./types";
import {
  createPasskeyAssertion,
  createPasskeyAttestation,
  serializePasskeyAssertion,
  serializePasskeyAttestation,
} from "./webauthn";

const REQUEST_OPTIONS_FIXTURE: PublicKeyCredentialRequestOptionsJSON = {
  challenge: "Y2hhbGxlbmdl",
  allowCredentials: [{ id: "Y3JlZGVudGlhbA", type: "public-key" }],
};

const CREATION_OPTIONS_FIXTURE: PublicKeyCredentialCreationOptionsJSON = {
  challenge: "Y2hhbGxlbmdl",
  rp: { id: "localhost", name: "Allyourbase" },
  user: { id: "dXNlci1pZA", name: "user@example.com", displayName: "User" },
  pubKeyCredParams: [{ type: "public-key", alg: -7 }],
  excludeCredentials: [{ id: "ZXhpc3RpbmctY3JlZA", type: "public-key" }],
};

function utf8Bytes(value: string): ArrayBuffer {
  return new TextEncoder().encode(value).buffer;
}

describe("createPasskeyAssertion", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the unsupported-browser error when PublicKeyCredential is missing", async () => {
    const credentialsGet = vi.fn();
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });
    vi.stubGlobal("PublicKeyCredential", undefined);
    vi.stubGlobal("AuthenticatorAssertionResponse", class MockAuthenticatorAssertionResponse {});
    vi.stubGlobal("AuthenticatorAttestationResponse", class MockAuthenticatorAttestationResponse {});

    await expect(createPasskeyAssertion(REQUEST_OPTIONS_FIXTURE)).rejects.toThrow(
      "This browser does not support WebAuthn credentials",
    );
    expect(credentialsGet).not.toHaveBeenCalled();
  });

  it("returns the unsupported-browser error when AuthenticatorAssertionResponse is missing", async () => {
    const credentialsGet = vi.fn();
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });
    vi.stubGlobal("PublicKeyCredential", class MockPublicKeyCredential {});
    vi.stubGlobal("AuthenticatorAssertionResponse", undefined);
    vi.stubGlobal("AuthenticatorAttestationResponse", class MockAuthenticatorAttestationResponse {});

    await expect(createPasskeyAssertion(REQUEST_OPTIONS_FIXTURE)).rejects.toThrow(
      "This browser does not support WebAuthn credentials",
    );
    expect(credentialsGet).not.toHaveBeenCalled();
  });

  it("serializes discoverable-login assertions without allowCredentials", async () => {
    class MockAssertion {
      clientDataJSON: ArrayBuffer;
      authenticatorData: ArrayBuffer;
      signature: ArrayBuffer;
      userHandle: ArrayBuffer | null;
      constructor(
        clientDataJSON: ArrayBuffer,
        authenticatorData: ArrayBuffer,
        signature: ArrayBuffer,
        userHandle: ArrayBuffer | null,
      ) {
        this.clientDataJSON = clientDataJSON;
        this.authenticatorData = authenticatorData;
        this.signature = signature;
        this.userHandle = userHandle;
      }
    }

    class MockPublicKeyCredential {
      id: string;
      rawId: ArrayBuffer;
      type: string;
      response: MockAssertion;
      constructor(id: string, rawId: ArrayBuffer, type: string, response: MockAssertion) {
        this.id = id;
        this.rawId = rawId;
        this.type = type;
        this.response = response;
      }
      getClientExtensionResults() {
        return { appid: false };
      }
    }

    const assertionResponse = new MockAssertion(
      utf8Bytes('{"type":"webauthn.get"}'),
      utf8Bytes("authenticator-data"),
      utf8Bytes("signature-bytes"),
      utf8Bytes("user-handle"),
    );
    const credential = new MockPublicKeyCredential(
      "discoverable-credential",
      utf8Bytes("raw-discoverable-id"),
      "public-key",
      assertionResponse,
    );
    const credentialsGet = vi.fn().mockResolvedValue(credential);
    vi.stubGlobal("navigator", { credentials: { get: credentialsGet } });
    vi.stubGlobal("PublicKeyCredential", MockPublicKeyCredential);
    vi.stubGlobal("AuthenticatorAssertionResponse", MockAssertion);
    vi.stubGlobal("AuthenticatorAttestationResponse", class MockAuthenticatorAttestationResponse {});

    const discoverableOptions: PublicKeyCredentialRequestOptionsJSON = {
      challenge: "ZGlzY292ZXJhYmxlLWNoYWxsZW5nZQ",
      rpId: "localhost",
      userVerification: "preferred",
    };

    const serialized = await createPasskeyAssertion(discoverableOptions);

    expect(credentialsGet).toHaveBeenCalledWith({
      publicKey: {
        challenge: utf8Bytes("discoverable-challenge"),
        rpId: "localhost",
        userVerification: "preferred",
        allowCredentials: undefined,
      },
    });
    expect(serialized).toEqual(
      serializePasskeyAssertion(
        credential as unknown as PublicKeyCredential,
        MockAssertion as unknown as typeof AuthenticatorAssertionResponse,
      ),
    );
    expect(serialized).toEqual({
      id: "discoverable-credential",
      rawId: "cmF3LWRpc2NvdmVyYWJsZS1pZA",
      type: "public-key",
      response: {
        clientDataJSON: "eyJ0eXBlIjoid2ViYXV0aG4uZ2V0In0",
        authenticatorData: "YXV0aGVudGljYXRvci1kYXRh",
        signature: "c2lnbmF0dXJlLWJ5dGVz",
        userHandle: "dXNlci1oYW5kbGU",
      },
      clientExtensionResults: { appid: false },
    });
  });
});

describe("createPasskeyAttestation", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the unsupported-browser error when PublicKeyCredential is missing", async () => {
    const credentialsCreate = vi.fn();
    vi.stubGlobal("navigator", { credentials: { create: credentialsCreate } });
    vi.stubGlobal("PublicKeyCredential", undefined);
    vi.stubGlobal("AuthenticatorAttestationResponse", class MockAuthenticatorAttestationResponse {});

    await expect(createPasskeyAttestation(CREATION_OPTIONS_FIXTURE)).rejects.toThrow(
      "This browser does not support WebAuthn credentials",
    );
    expect(credentialsCreate).not.toHaveBeenCalled();
  });

  it("returns the unsupported-browser error when AuthenticatorAttestationResponse is missing", async () => {
    const credentialsCreate = vi.fn();
    vi.stubGlobal("navigator", { credentials: { create: credentialsCreate } });
    vi.stubGlobal("PublicKeyCredential", class MockPublicKeyCredential {});
    vi.stubGlobal("AuthenticatorAttestationResponse", undefined);

    await expect(createPasskeyAttestation(CREATION_OPTIONS_FIXTURE)).rejects.toThrow(
      "This browser does not support WebAuthn credentials",
    );
    expect(credentialsCreate).not.toHaveBeenCalled();
  });

  it("serializes attestation response with base64url client data and attestation object", () => {
    class MockAttestation {
      clientDataJSON: ArrayBuffer;
      attestationObject: ArrayBuffer;
      constructor(clientDataJSON: ArrayBuffer, attestationObject: ArrayBuffer) {
        this.clientDataJSON = clientDataJSON;
        this.attestationObject = attestationObject;
      }
    }

    const clientData = utf8Bytes('{"type":"webauthn.create"}');
    const attestationObject = utf8Bytes("attestation-bytes");
    const rawId = utf8Bytes("raw-id");
    const response = new MockAttestation(clientData, attestationObject);
    const credential = {
      id: "credential-id",
      rawId,
      type: "public-key",
      response,
      getClientExtensionResults: () => ({}),
    } as unknown as PublicKeyCredential;

    const serialized = serializePasskeyAttestation(
      credential,
      MockAttestation as unknown as typeof AuthenticatorAttestationResponse,
    );

    expect(serialized).toEqual({
      id: "credential-id",
      rawId: "cmF3LWlk",
      type: "public-key",
      response: {
        clientDataJSON: "eyJ0eXBlIjoid2ViYXV0aG4uY3JlYXRlIn0",
        attestationObject: "YXR0ZXN0YXRpb24tYnl0ZXM",
      },
      clientExtensionResults: {},
    });
  });
});
