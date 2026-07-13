import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
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
import type { ApiError } from "../api_client";

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
    vi.spyOn(passkeyApi, "listPasskeys").mockResolvedValue([]);
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

  it("renders credential metadata and deletes by credential id", async () => {
    const user = userEvent.setup();
    const factors: MFAFactor[] = [
      {
        id: "factor-1",
        method: "webauthn",
        label: "Fallback Label",
        display_name: "MacBook Touch ID",
      },
    ];

    vi.spyOn(passkeyApi, "listPasskeys").mockResolvedValue([
      {
        credentialId: "credential/a?b",
        displayName: "MacBook Touch ID",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    const deleteSpy = vi.spyOn(passkeyApi, "deletePasskey").mockResolvedValue();

    renderWithProviders(<Passkeys factors={factors} onChanged={onChanged} />);

    expect(await screen.findByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");

    await user.click(screen.getByTestId("passkey-delete-button"));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledTimes(1);
    });
    expect(deleteSpy).toHaveBeenCalledWith("credential/a?b");
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
  });

  it("renames a credential by credential id through the component owner", async () => {
    const user = userEvent.setup();
    vi.spyOn(passkeyApi, "listPasskeys")
      .mockResolvedValueOnce([
        {
          credentialId: "credential/a?b",
          displayName: "MacBook Touch ID",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
      ])
      .mockResolvedValueOnce([
        {
          credentialId: "credential/a?b",
          displayName: "Renamed work key",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
      ]);
    const renameSpy = vi.spyOn(passkeyApi, "renamePasskey").mockResolvedValue({
      credentialId: "credential/a?b",
      displayName: "Renamed work key",
      transports: ["internal"],
      createdAt: "2026-07-12T12:00:00Z",
    });

    renderWithProviders(<Passkeys factors={[]} onChanged={onChanged} />);
    const row = await screen.findByTestId("passkey-row-credential/a?b");

    await user.clear(within(row).getByTestId("passkey-rename-input"));
    await user.type(within(row).getByTestId("passkey-rename-input"), " Renamed work key ");
    await user.click(within(row).getByTestId("passkey-rename-button"));

    await waitFor(() => {
      expect(renameSpy).toHaveBeenCalledWith("credential/a?b", "Renamed work key");
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledTimes(1);
    });
    expect(await screen.findByText("Renamed work key")).toBeInTheDocument();
  });

  it("keeps a final credential visible when delete is rejected by the backend", async () => {
    const user = userEvent.setup();
    vi.spyOn(passkeyApi, "listPasskeys").mockResolvedValue([
      {
        credentialId: "credential-final",
        displayName: "Only passkey",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    vi.spyOn(passkeyApi, "deletePasskey").mockRejectedValue(
      new Error("cannot delete final WebAuthn credential"),
    );

    renderWithProviders(<Passkeys factors={[]} onChanged={onChanged} />);
    await user.click(await screen.findByTestId("passkey-delete-button"));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /delete/i }));

    expect(await screen.findByText("cannot delete final WebAuthn credential")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-row-credential-final")).toBeVisible();
    expect(onChanged).not.toHaveBeenCalled();
  });
});

describe("dashboard passkey credential transport", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
    fetchMock.mockReset();
    localStorage.clear();
    vi.stubGlobal("fetch", fetchMock);
    api.setAuthToken("session-token");
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function jsonResponse(body: unknown, init?: ResponseInit): Response {
    return new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
      ...init,
    });
  }

  it("lists passkey credentials from the pinned credential-management envelope", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      credentials: [
        {
          credential_id: "credential/a?b",
          display_name: "Primary laptop",
          transports: ["internal", "hybrid"],
          created_at: "2026-07-10T12:00:00Z",
          last_used_at: "2026-07-11T12:00:00Z",
          id: "database-row-id",
          factor_id: "factor-id",
          public_key: "public-key-material",
          sign_count: 9,
        },
        {
          credential_id: "backup-key",
          display_name: "Backup key",
          transports: [],
          created_at: "2026-07-09T12:00:00Z",
        },
      ],
    }));

    const result = await passkeyApi.listPasskeys();
    const [, init] = fetchMock.mock.calls[0];

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/mfa/webauthn/credentials",
      expect.objectContaining({
        method: "GET",
        headers: expect.objectContaining({ Authorization: "Bearer session-token" }),
      }),
    );
    expect(init?.method).toBe("GET");
    expect(result).toEqual([
      {
        credentialId: "credential/a?b",
        displayName: "Primary laptop",
        transports: ["internal", "hybrid"],
        createdAt: "2026-07-10T12:00:00Z",
        lastUsedAt: "2026-07-11T12:00:00Z",
      },
      {
        credentialId: "backup-key",
        displayName: "Backup key",
        transports: [],
        createdAt: "2026-07-09T12:00:00Z",
      },
    ]);
    expect(Object.keys(result[0])).toEqual([
      "credentialId",
      "displayName",
      "transports",
      "createdAt",
      "lastUsedAt",
    ]);
  });

  it.each([
    [{ factors: [] }],
    [{ credentials: {} }],
  ])("rejects malformed passkey credential list envelopes: %j", async (body) => {
    fetchMock.mockResolvedValueOnce(jsonResponse(body));

    await expect(passkeyApi.listPasskeys()).rejects.toThrow(
      "Invalid passkey credentials response",
    );
  });

  it("rejects passkey credential metadata without a transports array", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      credentials: [
        {
          credential_id: "credential-without-transports",
          display_name: "Missing transports",
          created_at: "2026-07-10T12:00:00Z",
        },
      ],
    }));

    await expect(passkeyApi.listPasskeys()).rejects.toThrow(
      "Invalid passkey credential metadata",
    );
  });

  it("renames an encoded passkey credential and normalizes the returned metadata", async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse({
      credential_id: "credential/a?b",
      display_name: "Renamed primary key",
      transports: ["usb"],
      created_at: "2026-07-10T12:00:00Z",
      id: "database-row-id",
      sign_count: 4,
    }));

    const result = await passkeyApi.renamePasskey("credential/a?b", "Renamed primary key");
    const [, init] = fetchMock.mock.calls[0];

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/mfa/webauthn/credentials/credential%2Fa%3Fb",
      expect.objectContaining({
        method: "PATCH",
        headers: expect.objectContaining({
          Authorization: "Bearer session-token",
          "Content-Type": "application/json",
        }),
        body: JSON.stringify({ display_name: "Renamed primary key" }),
      }),
    );
    expect(init?.body).toBe(JSON.stringify({ display_name: "Renamed primary key" }));
    expect(result).toEqual({
      credentialId: "credential/a?b",
      displayName: "Renamed primary key",
      transports: ["usb"],
      createdAt: "2026-07-10T12:00:00Z",
    });
  });

  it("deletes an encoded passkey credential without a request body", async () => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await passkeyApi.deletePasskey("credential/a?b");
    const [, init] = fetchMock.mock.calls[0];

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/auth/mfa/webauthn/credentials/credential%2Fa%3Fb",
      expect.objectContaining({
        method: "DELETE",
        headers: expect.objectContaining({ Authorization: "Bearer session-token" }),
      }),
    );
    expect(init?.body).toBeUndefined();
  });

  it.each([
    "cannot delete final WebAuthn credential",
    "MFA verification is required for this action",
  ])("propagates backend delete ApiError messages: %s", async (message) => {
    fetchMock.mockResolvedValueOnce(jsonResponse({ message }, { status: 403 }));

    await expect(passkeyApi.deletePasskey("credential-id")).rejects.toMatchObject({
      message,
    } satisfies Partial<ApiError>);
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
    vi.spyOn(passkeyApi, "listPasskeys").mockResolvedValue([]);
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

  it("does not show AAL2 after passkey registration unless the auth token is upgraded", async () => {
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

    expect(await screen.findByText('Passkey "Laptop Key" registered')).toBeInTheDocument();
    expect(screen.getByTestId("aal-level-indicator")).toHaveTextContent(/AAL1/i);
  });
});
