import { describe, expect, it, vi } from "vitest";
import type { BrowserContext, CDPSession, Page } from "@playwright/test";
import { createVirtualAuthenticator } from "../../browser-tests-unmocked/fixtures";

describe("browser-unmocked auth fixture helpers", () => {
  it("creates and tears down a CDP virtual authenticator via Chromium context session", async () => {
    const send = vi
      .fn<CDPSession["send"]>()
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({ authenticatorId: "virt-auth-1" })
      .mockResolvedValueOnce(undefined)
      .mockResolvedValue(undefined);

    const detach = vi.fn<CDPSession["detach"]>().mockResolvedValue(undefined);
    const cdpSession = { send, detach } as unknown as CDPSession;

    const newCDPSession = vi.fn(async () => cdpSession);
    const context = {
      newCDPSession,
    } as unknown as BrowserContext;
    const page = {
      context: () => context,
    } as unknown as Page;

    const authenticator = await createVirtualAuthenticator(page);

    expect(newCDPSession).toHaveBeenCalledTimes(1);
    expect(send).toHaveBeenNthCalledWith(1, "WebAuthn.enable");
    expect(send).toHaveBeenNthCalledWith(
      2,
      "WebAuthn.addVirtualAuthenticator",
      expect.objectContaining({
        options: expect.objectContaining({
          protocol: "ctap2",
          transport: "internal",
          hasResidentKey: true,
          hasUserVerification: true,
          isUserVerified: true,
          automaticPresenceSimulation: true,
        }),
      }),
    );

    await authenticator.remove();

    expect(send).toHaveBeenNthCalledWith(3, "WebAuthn.removeVirtualAuthenticator", {
      authenticatorId: "virt-auth-1",
    });
    expect(send).toHaveBeenNthCalledWith(4, "WebAuthn.disable");
    expect(detach).toHaveBeenCalledTimes(1);
  });

  it("throws and cleans up CDP session when virtual authenticator setup omits authenticatorId", async () => {
    const send = vi
      .fn<CDPSession["send"]>()
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce(undefined);
    const detach = vi.fn<CDPSession["detach"]>().mockResolvedValue(undefined);
    const cdpSession = { send, detach } as unknown as CDPSession;
    const newCDPSession = vi.fn(async () => cdpSession);
    const context = {
      newCDPSession,
    } as unknown as BrowserContext;
    const page = {
      context: () => context,
    } as unknown as Page;

    await expect(createVirtualAuthenticator(page)).rejects.toThrow(
      "CDP virtual authenticator setup succeeded but no authenticatorId was returned",
    );

    expect(send).toHaveBeenNthCalledWith(1, "WebAuthn.enable");
    expect(send).toHaveBeenNthCalledWith(
      2,
      "WebAuthn.addVirtualAuthenticator",
      expect.objectContaining({
        options: expect.objectContaining({
          protocol: "ctap2",
          transport: "internal",
          hasResidentKey: true,
          hasUserVerification: true,
          isUserVerified: true,
          automaticPresenceSimulation: true,
        }),
      }),
    );
    expect(send).toHaveBeenNthCalledWith(3, "WebAuthn.disable");
    expect(detach).toHaveBeenCalledTimes(1);
  });

  it("swallows teardown command errors so virtual authenticator cleanup is best-effort", async () => {
    const send = vi
      .fn<CDPSession["send"]>()
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce({ authenticatorId: "virt-auth-2" })
      .mockRejectedValueOnce(new Error("remove failed"))
      .mockRejectedValueOnce(new Error("disable failed"));
    const detach = vi.fn<CDPSession["detach"]>().mockRejectedValue(new Error("detach failed"));
    const cdpSession = { send, detach } as unknown as CDPSession;
    const newCDPSession = vi.fn(async () => cdpSession);
    const context = {
      newCDPSession,
    } as unknown as BrowserContext;
    const page = {
      context: () => context,
    } as unknown as Page;

    const authenticator = await createVirtualAuthenticator(page);

    await expect(authenticator.remove()).resolves.toBeUndefined();
    expect(send).toHaveBeenNthCalledWith(3, "WebAuthn.removeVirtualAuthenticator", {
      authenticatorId: "virt-auth-2",
    });
    expect(send).toHaveBeenNthCalledWith(4, "WebAuthn.disable");
    expect(detach).toHaveBeenCalledTimes(1);
  });
});
