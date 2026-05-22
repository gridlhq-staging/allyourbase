import { describe, expect, it } from "vitest";
import {
  asRecord,
  encodePathSegment,
  encodePathWithSlashes,
  normalizeAuthResponse,
  normalizeMagicLinkConfirmResponse,
  normalizeRealtimeEvent,
  normalizeStorageListResponse,
  normalizeUser,
} from "./helpers";

describe("sdk helpers", () => {
  it("encodes URL path segments safely", () => {
    expect(encodePathSegment("posts/admin users")).toBe("posts%2Fadmin%20users");
    expect(encodePathWithSlashes("public/posts and comments")).toBe("public/posts%20and%20comments");
  });

  it("normalizes auth response and user fields", () => {
    const normalized = normalizeAuthResponse({
      token: "t",
      refreshToken: "r",
      user: {
        id: 7,
        email: "demo@example.com",
        email_verified: true,
        created_at: "2026-03-01T00:00:00Z",
      } as unknown,
    } as unknown);

    expect(normalized.token).toBe("t");
    expect(normalized.refreshToken).toBe("r");
    expect(normalized.user.id).toBe("7");
    expect(normalized.user.emailVerified).toBe(true);
    expect(normalized.user.createdAt).toBe("2026-03-01T00:00:00Z");
  });

  it("normalizes anonymous user fields into canonical SDK shape", () => {
    const normalized = normalizeUser({
      id: "anon-1",
      is_anonymous: true,
      linked_at: "2026-05-20T10:11:12Z",
      phone: "+15555550123",
      createdAt: "2026-05-20T09:00:00Z",
      updatedAt: "2026-05-20T10:00:00Z",
    } as unknown);

    expect(normalized.id).toBe("anon-1");
    expect(normalized.email).toBeUndefined();
    expect(normalized.isAnonymous).toBe(true);
    expect(normalized.linkedAt).toBe("2026-05-20T10:11:12Z");
    expect(normalized.phone).toBe("+15555550123");
    expect(normalized).not.toHaveProperty("is_anonymous");
    expect(normalized).not.toHaveProperty("linked_at");
  });

  it("normalizes backend empty-string sentinels for anonymous optional fields", () => {
    const normalized = normalizeUser({
      id: "anon-2",
      email: "",
      phone: "",
      is_anonymous: true,
      linked_at: "2026-05-20T10:11:12Z",
      created_at: "2026-05-20T09:00:00Z",
      updated_at: "2026-05-20T10:00:00Z",
    } as unknown);

    expect(normalized.id).toBe("anon-2");
    expect(normalized.email).toBeUndefined();
    expect(normalized.phone).toBeUndefined();
    expect(normalized.isAnonymous).toBe(true);
    expect(normalized.linkedAt).toBe("2026-05-20T10:11:12Z");
  });

  it("normalizes magic-link confirm full auth response", () => {
    const normalized = normalizeMagicLinkConfirmResponse({
      token: "tok",
      refreshToken: "ref",
      user: {
        id: "1",
        email: "demo@example.com",
        email_verified: true,
      },
    } as unknown);

    expect("token" in normalized).toBe(true);
    if ("token" in normalized) {
      expect(normalized.user.emailVerified).toBe(true);
      expect(normalized.token).toBe("tok");
      expect(normalized.refreshToken).toBe("ref");
    }
  });

  it("normalizes magic-link confirm MFA pending response", () => {
    const normalized = normalizeMagicLinkConfirmResponse({
      mfa_pending: true,
      mfa_token: "pending-token",
    } as unknown);
    expect(normalized).toEqual({
      mfaPending: true,
      mfaToken: "pending-token",
    });
  });

  it("normalizes storage list defaults and realtime old_record", () => {
    const storage = normalizeStorageListResponse({
      items: [{ id: "1", bucket: "b", name: "n", size: 9, content_type: "text/plain" }] as never,
      totalItems: undefined as never,
    });
    expect(storage.totalItems).toBe(1);
    expect(storage.items[0].contentType).toBe("text/plain");

    const event = normalizeRealtimeEvent({
      action: "update",
      table: "posts",
      record: { id: "2" },
      old_record: { id: "1" },
    } as unknown);
    expect(event.oldRecord).toEqual({ id: "1" });
  });

  it("asRecord returns undefined for non-object values", () => {
    expect(asRecord(null)).toBeUndefined();
    expect(asRecord("x")).toBeUndefined();
    expect(asRecord({ ok: true })).toEqual({ ok: true });
  });
});
