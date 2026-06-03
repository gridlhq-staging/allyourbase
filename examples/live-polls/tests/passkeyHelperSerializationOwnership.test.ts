import { describe, expect, it } from "vitest";
import helperSource from "../e2e/helpers.ts?raw";

describe("passkey helper serialization ownership", () => {
  it("avoids helper-local base64url attestation shaping and relies on credential JSON serialization", () => {
    expect(helperSource.includes("const encodeBase64URL = (buffer: ArrayBuffer): string => {")).toBe(false);
    expect(helperSource.includes("attestationObject: encodeBase64URL(response.attestationObject)")).toBe(false);
    expect(helperSource.includes("const toJSON = (credential as { toJSON?: () => unknown }).toJSON;")).toBe(true);
    expect(helperSource.includes("const attestationResponse = toJSON.call(credential);")).toBe(true);
  });
});
