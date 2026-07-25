import { describe, expect, it, vi } from "vitest";
import type { APIRequestContext } from "@playwright/test";
import { seedSAMLProvider } from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown, statusCode = 200) {
  return {
    ok: () => statusCode >= 200 && statusCode < 300,
    status: () => statusCode,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

function buildInfraRequestMock() {
  const posts: Array<{ path: string; data?: unknown; headers?: unknown }> = [];

  const request = {
    post: vi.fn(async (path: string, init?: { data?: unknown; headers?: unknown }) => {
      posts.push({ path, data: init?.data, headers: init?.headers });
      if (path === "/api/admin/auth/saml") {
        return okResponse({});
      }
      throw new Error(`Unexpected POST ${path}`);
    }),
  };

  return { request: request as unknown as APIRequestContext, posts };
}

describe("browser-unmocked infra fixture helpers", () => {
  it("seeds SAML providers with signing-certificate metadata by default", async () => {
    const { request, posts } = buildInfraRequestMock();

    await seedSAMLProvider(request, "admin-token", {
      name: "smoke-saml-runone",
      entity_id: "urn:smoke:runone",
    });

    expect(posts[0]).toMatchObject({
      path: "/api/admin/auth/saml",
      headers: { Authorization: "Bearer admin-token", "Content-Type": "application/json" },
      data: {
        name: "smoke-saml-runone",
        entity_id: "urn:smoke:runone",
      },
    });

    const payload = posts[0]?.data as { idp_metadata_xml?: string };
    expect(payload.idp_metadata_xml).toContain('<KeyDescriptor use="signing">');
    expect(payload.idp_metadata_xml).toMatch(
      /<X509Certificate>[A-Za-z0-9+/=\s]+<\/X509Certificate>/,
    );
    expect(payload.idp_metadata_xml).toContain("https://idp.example.test/smoke-saml-runone/sso");
  });
});
