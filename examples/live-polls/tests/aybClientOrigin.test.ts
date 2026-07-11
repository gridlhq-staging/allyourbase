import { describe, expect, it, vi } from "vitest";

const mockAYBClient = vi.hoisted(() =>
  vi.fn(() => ({
    clearTokens: vi.fn(),
  })),
);

vi.mock("@allyourbase/js", () => ({
  AYBClient: mockAYBClient,
}));

describe("live-polls AYB client origin", () => {
  it("defaults to the hosted page origin when VITE_AYB_URL is not configured", async () => {
    vi.resetModules();
    mockAYBClient.mockClear();

    await import("../src/lib/ayb");

    expect(mockAYBClient).toHaveBeenCalledWith(
      window.location.origin,
      expect.objectContaining({
        authPersistence: expect.any(Object),
      }),
    );
  });
});
