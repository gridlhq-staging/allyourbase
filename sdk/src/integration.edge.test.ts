import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { beforeAll, describe, expect, it } from "vitest";
import { AYBClient } from "./client";
import { createTestClient, primeIntegrationSuite } from "./integration-helpers";

// The `sdk-contract-echo` edge function and these fixtures are the deterministic
// Stage 2 SDK contract specimen, seeded live by scripts/sdk_live_proof_seed.sh
// before the integration lane runs.
const CONTRACT_FIXTURE_DIR = resolve(
  __dirname,
  "../../tests/contract/fixtures/sdk_contract",
);

function fixtureText(name: string): string {
  return readFileSync(resolve(CONTRACT_FIXTURE_DIR, name), "utf8");
}

describe("SDK edge invoke live proof", () => {
  let client: AYBClient;
  const requestBody = JSON.parse(fixtureText("edge_invoke_request.json")) as {
    message: string;
  };
  // Compact wire bytes the server emits, reconstructed from the on-disk fixture.
  const expectedRawBody = JSON.stringify(
    JSON.parse(fixtureText("edge_invoke_response.json")),
  );

  beforeAll(async () => {
    await primeIntegrationSuite();
    client = createTestClient();
  }, 35_000);

  it("invokes sdk-contract-echo and returns the captured status and raw body", async () => {
    const res = await client.functions.invoke("sdk-contract-echo", {
      body: requestBody,
    });

    expect(res.status).toBe(200);
    expect(res.rawBody).toBe(expectedRawBody);
    expect(JSON.parse(res.rawBody)).toEqual({
      message: requestBody.message,
      method: "POST",
      specimen: "sdk-contract-echo",
    });
  });
});
