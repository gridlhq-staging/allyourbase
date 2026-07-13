import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Passkeys } from "../Passkeys";
import {
  beginPasskeyEnroll,
  confirmPasskeyEnroll,
  deletePasskey,
  listPasskeys,
  renamePasskey,
} from "../../api_passkeys";
import { createPasskeyAttestation } from "../../webauthn";
import type { MFAFactor } from "../../types";

vi.mock("../../api_passkeys", () => ({
  beginPasskeyEnroll: vi.fn(),
  confirmPasskeyEnroll: vi.fn(),
  deletePasskey: vi.fn(),
  listPasskeys: vi.fn(),
  renamePasskey: vi.fn(),
}));

vi.mock("../../webauthn", () => ({
  createPasskeyAttestation: vi.fn(),
}));

const mockBeginPasskeyEnroll = vi.mocked(beginPasskeyEnroll);
const mockConfirmPasskeyEnroll = vi.mocked(confirmPasskeyEnroll);
const mockDeletePasskey = vi.mocked(deletePasskey);
const mockListPasskeys = vi.mocked(listPasskeys);
const mockRenamePasskey = vi.mocked(renamePasskey);
const mockCreatePasskeyAttestation = vi.mocked(createPasskeyAttestation);

const PASSKEY_FACTOR: MFAFactor = {
  id: "factor-passkey-1",
  method: "webauthn",
  label: "MacBook Touch ID",
  display_name: "MacBook Touch ID",
};

describe("Passkeys", () => {
  const onChanged = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockListPasskeys.mockResolvedValue([]);
  });

  it("renders passkey display names from credential metadata", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-passkey-1",
        displayName: "MacBook Touch ID",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    expect(await screen.findByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");
  });

  it("surfaces loading, empty, and load failure states", async () => {
    let resolveList: (credentials: []) => void = () => {};
    mockListPasskeys.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveList = resolve;
      }),
    );

    render(<Passkeys factors={[]} onChanged={onChanged} />);

    expect(screen.getByText("Loading passkeys...")).toBeInTheDocument();
    resolveList([]);
    expect(await screen.findByText("No passkeys registered")).toBeInTheDocument();

    mockListPasskeys.mockRejectedValueOnce(new Error("passkey list unavailable"));
    render(<Passkeys factors={[]} onChanged={onChanged} />);

    expect(await screen.findByText("passkey list unavailable")).toBeInTheDocument();
  });

  it("renders one metadata row per credential with formatted timestamps", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-primary",
        displayName: "Primary laptop",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
        lastUsedAt: "2026-07-13T15:30:00Z",
      },
      {
        credentialId: "credential-backup",
        displayName: "Backup security key",
        transports: ["usb"],
        createdAt: "2026-07-10T09:15:00Z",
      },
    ]);

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);

    const rows = await screen.findAllByTestId(/^passkey-row-/);
    expect(rows).toHaveLength(2);
    expect(within(rows[0]).getByTestId("passkey-name")).toHaveTextContent("Primary laptop");
    expect(within(rows[0]).getByText(/Created Jul 12, 2026/)).toBeInTheDocument();
    expect(within(rows[0]).getByText(/Last used Jul 13, 2026/)).toBeInTheDocument();
    expect(within(rows[1]).getByTestId("passkey-name")).toHaveTextContent("Backup security key");
    expect(within(rows[1]).getByText(/Created Jul 10, 2026/)).toBeInTheDocument();
    expect(within(rows[1]).queryByText(/Last used/)).not.toBeInTheDocument();
  });

  it("registers a passkey with the browser attestation payload", async () => {
    mockBeginPasskeyEnroll.mockResolvedValue({
      challenge: "Y2hhbGxlbmdl",
      rp: { id: "localhost", name: "Allyourbase" },
      user: {
        id: "dXNlci0x",
        name: "user@example.com",
        displayName: "user@example.com",
      },
      pubKeyCredParams: [{ alg: -7, type: "public-key" }],
    });
    mockCreatePasskeyAttestation.mockResolvedValue({ id: "cred-1" });
    mockConfirmPasskeyEnroll.mockResolvedValue({ message: "ok" });

    const user = userEvent.setup();
    render(<Passkeys factors={[]} onChanged={onChanged} />);

    await user.type(screen.getByTestId("passkey-display-name-input"), "MacBook Touch ID");
    await user.click(screen.getByTestId("passkey-register-button"));

    await waitFor(() => {
      expect(mockBeginPasskeyEnroll).toHaveBeenCalledOnce();
    });
    await waitFor(() => {
      expect(mockCreatePasskeyAttestation).toHaveBeenCalledOnce();
    });
    await waitFor(() => {
      expect(mockConfirmPasskeyEnroll).toHaveBeenCalledWith("MacBook Touch ID", { id: "cred-1" });
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledOnce();
    });
  });

  it("adds another credential through enrollment and refreshes the list", async () => {
    mockListPasskeys
      .mockResolvedValueOnce([
        {
          credentialId: "credential-primary",
          displayName: "Primary laptop",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
      ])
      .mockResolvedValueOnce([
        {
          credentialId: "credential-primary",
          displayName: "Primary laptop",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
        {
          credentialId: "credential-backup",
          displayName: "Backup key",
          transports: ["usb"],
          createdAt: "2026-07-13T12:00:00Z",
        },
      ]);
    mockBeginPasskeyEnroll.mockResolvedValue({
      challenge: "Y2hhbGxlbmdl",
      rp: { id: "localhost", name: "Allyourbase" },
      user: {
        id: "dXNlci0x",
        name: "user@example.com",
        displayName: "user@example.com",
      },
      pubKeyCredParams: [{ alg: -7, type: "public-key" }],
    });
    mockCreatePasskeyAttestation.mockResolvedValue({ id: "cred-2" });
    mockConfirmPasskeyEnroll.mockResolvedValue({ message: "ok" });
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    expect(await screen.findByText("Primary laptop")).toBeInTheDocument();

    await user.type(screen.getByTestId("passkey-display-name-input"), "Backup key");
    await user.click(screen.getByTestId("passkey-register-button"));

    expect(await screen.findByText("Backup key")).toBeInTheDocument();
    expect(screen.getAllByTestId(/^passkey-row-/)).toHaveLength(2);
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledOnce();
    });
  });

  it("renames one credential inline and refreshes through the list owner", async () => {
    mockListPasskeys
      .mockResolvedValueOnce([
        {
          credentialId: "credential-primary",
          displayName: "Primary laptop",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
        {
          credentialId: "credential-backup",
          displayName: "Backup key",
          transports: ["usb"],
          createdAt: "2026-07-10T12:00:00Z",
        },
      ])
      .mockResolvedValueOnce([
        {
          credentialId: "credential-primary",
          displayName: "Renamed laptop",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
        {
          credentialId: "credential-backup",
          displayName: "Backup key",
          transports: ["usb"],
          createdAt: "2026-07-10T12:00:00Z",
        },
      ]);
    mockRenamePasskey.mockResolvedValue({
      credentialId: "credential-primary",
      displayName: "Renamed laptop",
      transports: ["internal"],
      createdAt: "2026-07-12T12:00:00Z",
    });
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    const row = await screen.findByTestId("passkey-row-credential-primary");

    await user.clear(within(row).getByTestId("passkey-rename-input"));
    await user.type(within(row).getByTestId("passkey-rename-input"), " Renamed laptop ");
    await user.click(within(row).getByTestId("passkey-rename-button"));

    await waitFor(() => {
      expect(mockRenamePasskey).toHaveBeenCalledWith("credential-primary", "Renamed laptop");
    });
    expect(await screen.findByText('Passkey "Renamed laptop" renamed')).toBeInTheDocument();
    expect(screen.getByText("Backup key")).toBeInTheDocument();
    expect(onChanged).toHaveBeenCalledOnce();
  });

  it("keeps the row visible and preserves backend rename errors", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-primary",
        displayName: "Primary laptop",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    mockRenamePasskey.mockRejectedValue(new Error("display name already exists"));
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    const row = await screen.findByTestId("passkey-row-credential-primary");

    await user.clear(within(row).getByTestId("passkey-rename-input"));
    await user.type(within(row).getByTestId("passkey-rename-input"), "Primary laptop");
    await user.click(within(row).getByTestId("passkey-rename-button"));

    expect(await screen.findByText("display name already exists")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-row-credential-primary")).toBeVisible();
  });

  it("deletes the enrolled passkey through the API client", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-passkey-1",
        displayName: "MacBook Touch ID",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    mockDeletePasskey.mockResolvedValue();
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    await user.click(await screen.findByTestId("passkey-delete-button"));
    const dialog = await screen.findByRole("dialog", { name: /delete passkey/i });
    await user.click(within(dialog).getByRole("button", { name: /delete/i }));

    await waitFor(() => {
      expect(mockDeletePasskey).toHaveBeenCalledWith("credential-passkey-1");
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledOnce();
    });
  });

  it("cancels delete confirmation without calling the API", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-passkey-1",
        displayName: "MacBook Touch ID",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    await user.click(await screen.findByTestId("passkey-delete-button"));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /cancel/i }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    });
    expect(mockDeletePasskey).not.toHaveBeenCalled();
    expect(screen.getByTestId("passkey-row-credential-passkey-1")).toBeVisible();
  });

  it("preserves the final credential row and backend error after final-credential delete rejection", async () => {
    mockListPasskeys.mockResolvedValue([
      {
        credentialId: "credential-passkey-1",
        displayName: "MacBook Touch ID",
        transports: ["internal"],
        createdAt: "2026-07-12T12:00:00Z",
      },
    ]);
    mockDeletePasskey.mockRejectedValue(new Error("cannot delete final WebAuthn credential"));
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    await user.click(await screen.findByTestId("passkey-delete-button"));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /delete/i }));

    expect(await screen.findByText("cannot delete final WebAuthn credential")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-row-credential-passkey-1")).toBeVisible();
    expect(onChanged).not.toHaveBeenCalled();
  });

  it("keeps the last loaded passkeys visible when a post-delete refresh fails", async () => {
    mockListPasskeys
      .mockResolvedValueOnce([
        {
          credentialId: "credential-passkey-1",
          displayName: "MacBook Touch ID",
          transports: ["internal"],
          createdAt: "2026-07-12T12:00:00Z",
        },
      ])
      .mockRejectedValueOnce(new Error("refresh failed"));
    mockDeletePasskey.mockResolvedValue();
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);

    expect(await screen.findByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");
    await user.click(screen.getByTestId("passkey-delete-button"));
    await user.click(within(await screen.findByRole("dialog")).getByRole("button", { name: /delete/i }));

    expect(await screen.findByText("refresh failed")).toBeInTheDocument();
    expect(screen.getByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");
    expect(screen.queryByText("No passkeys registered")).not.toBeInTheDocument();
  });
});
