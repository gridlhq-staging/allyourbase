import { beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { MFAFactor, SchemaCache } from "../types";
import { renderWithProviders } from "../test-utils";
import { Passkeys } from "../components/Passkeys";
import { ContentRouter } from "../components/ContentRouter";
import { Sidebar } from "../components/Sidebar";
import { CommandPalette } from "../components/CommandPalette";
import * as passkeyApi from "../api_passkeys";
import * as webauthn from "../webauthn";
import * as api from "../api";

function buildAuthToken(payload: Record<string, unknown>): string {
  const header = Buffer.from(JSON.stringify({ alg: "HS256", typ: "JWT" })).toString("base64url");
  const body = Buffer.from(JSON.stringify(payload)).toString("base64url");
  return `${header}.${body}.signature`;
}

describe("Passkeys component", () => {
  const onChanged = vi.fn(async () => {});

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  it("rejects blank display name without calling passkey enrollment APIs", async () => {
    const user = userEvent.setup();
    const beginEnrollSpy = vi.spyOn(passkeyApi, "beginPasskeyEnroll");

    renderWithProviders(<Passkeys factors={[]} onChanged={onChanged} />);

    await user.click(screen.getByTestId("passkey-register-button"));

    expect(await screen.findByText("Passkey name is required")).toBeInTheDocument();
    expect(beginEnrollSpy).not.toHaveBeenCalled();
  });

  it("submits enroll and confirm requests, then refreshes factors", async () => {
    const user = userEvent.setup();
    const beginOptions = {
      challenge: "Y2hhbGxlbmdl",
      rp: { id: "localhost", name: "AYB" },
      user: { id: "dXNlcg", name: "user@example.com", displayName: "user@example.com" },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    };

    vi.spyOn(passkeyApi, "beginPasskeyEnroll").mockResolvedValue(beginOptions);
    vi.spyOn(webauthn, "createPasskeyAttestation").mockResolvedValue({ id: "attestation-1" });
    const confirmSpy = vi
      .spyOn(passkeyApi, "confirmPasskeyEnroll")
      .mockResolvedValue({ message: "ok" });
    const onPasskeyRegistered = vi.fn();

    renderWithProviders(
      <Passkeys factors={[]} onChanged={onChanged} onPasskeyRegistered={onPasskeyRegistered} />,
    );

    await user.type(screen.getByTestId("passkey-display-name-input"), "  Work Laptop Key  "
    );
    await user.click(screen.getByTestId("passkey-register-button"));

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledWith("Work Laptop Key", { id: "attestation-1" });
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
    expect(onPasskeyRegistered).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Passkey "Work Laptop Key" registered')).toBeInTheDocument();
    expect(screen.getByTestId("passkey-name")).toHaveTextContent("Work Laptop Key");
  });

  it("renders passkey display_name and allows single-delete endpoint usage", async () => {
    const user = userEvent.setup();
    const factors: MFAFactor[] = [
      {
        id: "factor-1",
        method: "webauthn",
        label: "Fallback Label",
        display_name: "MacBook Touch ID",
      },
    ];

    const deleteSpy = vi.spyOn(passkeyApi, "deletePasskey").mockResolvedValue();

    renderWithProviders(<Passkeys factors={factors} onChanged={onChanged} />);

    expect(screen.getByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");

    await user.click(screen.getByTestId("passkey-delete-button"));

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
  });
});

describe("MFA canonical passkey entry points", () => {
  const minimalSchema: SchemaCache = {
    tables: {},
    schemas: ["public"],
    builtAt: "2026-06-01T00:00:00Z",
  };

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
    vi.spyOn(api, "getMFAFactors").mockResolvedValue({ factors: [] });
    vi.spyOn(api, "getBackupCodeCount").mockResolvedValue({ remaining: 0 });
    vi.spyOn(api, "getAuthToken").mockReturnValue(null);
    vi.spyOn(api, "createAnonymousSession").mockResolvedValue({
      token: "anon-token",
      refreshToken: "anon-refresh",
      user: {
        id: "anon-user",
        email: "",
        is_anonymous: true,
        createdAt: "2026-06-01T00:00:00Z",
        updatedAt: "2026-06-01T00:00:00Z",
      },
    });
    vi.spyOn(api, "linkEmail").mockResolvedValue({
      token: "linked-token",
      refreshToken: "linked-refresh",
      user: {
        id: "linked-user",
        email: "linked@example.test",
        is_anonymous: false,
        createdAt: "2026-06-01T00:00:00Z",
        updatedAt: "2026-06-01T00:00:00Z",
      },
    });
  });

  it("renders passkey owner from ContentRouter for the mfa-management view", async () => {
    renderWithProviders(
      <ContentRouter
        schema={minimalSchema}
        view="mfa-management"
        isAdminView
        selected={null}
        onRefresh={async () => {}}
        onSetView={() => {}}
        onSelectAdminView={() => {}}
      />,
    );

    expect(await screen.findByRole("heading", { name: /multi-factor authentication/i })).toBeInTheDocument();
    expect(await screen.findByRole("heading", { name: /^passkeys$/i })).toBeInTheDocument();
  });

  it("routes sidebar and command palette entry points to the same mfa-management owner path", async () => {
    const user = userEvent.setup();
    const onSelectAdminView = vi.fn();
    const onSelect = vi.fn();
    const onClose = vi.fn();

    renderWithProviders(
      <>
        <Sidebar
          tables={[]}
          selected={null}
          view="users"
          isAdminView
          onSelectTable={() => {}}
          onSelectAdminView={onSelectAdminView}
          onOpenCommandPalette={() => {}}
          onRefresh={() => {}}
          onToggleTheme={() => {}}
          onLogout={() => {}}
          theme="light"
          themeToggleLabel="Switch to dark mode"
        />
        <CommandPalette open onClose={onClose} onSelect={onSelect} tables={[]} />
      </>,
    );

    const mfaEntryPoints = screen.getAllByRole("button", { name: /^MFA Management$/i });

    await user.click(mfaEntryPoints[0]);
    expect(onSelectAdminView).toHaveBeenCalledWith("mfa-management");

    await user.click(mfaEntryPoints[1]);
    expect(onSelect).toHaveBeenCalledWith({ kind: "view", view: "mfa-management" });
    expect(onClose).toHaveBeenCalled();
  });

  it("bootstraps a linked auth session for MFA when only an anonymous session exists", async () => {
    renderWithProviders(
      <ContentRouter
        schema={minimalSchema}
        view="mfa-management"
        isAdminView
        selected={null}
        onRefresh={async () => {}}
        onSetView={() => {}}
        onSelectAdminView={() => {}}
      />,
    );

    await waitFor(() => {
      expect(api.createAnonymousSession).toHaveBeenCalledTimes(1);
    });
    await waitFor(() => {
      expect(api.linkEmail).toHaveBeenCalledTimes(1);
    });
  });

  it("links an existing anonymous auth token instead of reusing it as an MFA-ready session", async () => {
    vi.spyOn(api, "getAuthToken").mockReturnValue(
      buildAuthToken({
        sub: "anon-user",
        is_anonymous: true,
        aal: "aal1",
      }),
    );

    renderWithProviders(
      <ContentRouter
        schema={minimalSchema}
        view="mfa-management"
        isAdminView
        selected={null}
        onRefresh={async () => {}}
        onSetView={() => {}}
        onSelectAdminView={() => {}}
      />,
    );

    await screen.findByRole("heading", { name: /multi-factor authentication/i });

    await waitFor(() => {
      expect(api.linkEmail).toHaveBeenCalledTimes(1);
    });
    expect(api.createAnonymousSession).not.toHaveBeenCalled();
  });

  it("keeps AAL2 indicator after successful passkey registration refresh", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "getAuthToken").mockReturnValue(null);
    vi.spyOn(api, "createAnonymousSession").mockResolvedValue({
      token: "anon-token",
      refreshToken: "anon-refresh",
      user: {
        id: "anon-user",
        email: "",
        is_anonymous: true,
        createdAt: "2026-06-01T00:00:00Z",
        updatedAt: "2026-06-01T00:00:00Z",
      },
    });
    vi.spyOn(api, "linkEmail").mockResolvedValue({
      token: "linked-token",
      refreshToken: "linked-refresh",
      user: {
        id: "linked-user",
        email: "linked@example.test",
        is_anonymous: false,
        createdAt: "2026-06-01T00:00:00Z",
        updatedAt: "2026-06-01T00:00:00Z",
      },
    });

    vi.spyOn(api, "getMFAFactors")
      .mockResolvedValueOnce({ factors: [] })
      .mockResolvedValueOnce({
        factors: [{ id: "factor-passkey-1", method: "webauthn", label: "Passkey", display_name: "Laptop Key" }],
      });
    vi.spyOn(api, "getBackupCodeCount")
      .mockResolvedValueOnce({ remaining: 0 })
      .mockResolvedValueOnce({ remaining: 0 });
    vi.spyOn(passkeyApi, "beginPasskeyEnroll").mockResolvedValue({
      challenge: "Y2hhbGxlbmdl",
      rp: { id: "localhost", name: "AYB" },
      user: { id: "dXNlcg", name: "user@example.com", displayName: "user@example.com" },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
    });
    vi.spyOn(webauthn, "createPasskeyAttestation").mockResolvedValue({ id: "attestation-1" });
    vi.spyOn(passkeyApi, "confirmPasskeyEnroll").mockResolvedValue({ message: "ok" });

    renderWithProviders(
      <ContentRouter
        schema={minimalSchema}
        view="mfa-management"
        isAdminView
        selected={null}
        onRefresh={async () => {}}
        onSetView={() => {}}
        onSelectAdminView={() => {}}
      />,
    );

    await screen.findByRole("heading", { name: /multi-factor authentication/i });

    await user.type(screen.getByTestId("passkey-display-name-input"), "Laptop Key");
    await user.click(screen.getByTestId("passkey-register-button"));

    await waitFor(() => {
      expect(screen.getByTestId("aal-level-indicator")).toHaveTextContent(/AAL2/i);
    });
  });
});
