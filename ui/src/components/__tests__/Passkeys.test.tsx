import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Passkeys } from "../Passkeys";
import {
  beginPasskeyEnroll,
  confirmPasskeyEnroll,
  deletePasskey,
} from "../../api_passkeys";
import { createPasskeyAttestation } from "../../webauthn";
import type { MFAFactor } from "../../types";

vi.mock("../../api_passkeys", () => ({
  beginPasskeyEnroll: vi.fn(),
  confirmPasskeyEnroll: vi.fn(),
  deletePasskey: vi.fn(),
}));

vi.mock("../../webauthn", () => ({
  createPasskeyAttestation: vi.fn(),
}));

const mockBeginPasskeyEnroll = vi.mocked(beginPasskeyEnroll);
const mockConfirmPasskeyEnroll = vi.mocked(confirmPasskeyEnroll);
const mockDeletePasskey = vi.mocked(deletePasskey);
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
  });

  it("renders passkey display names from factor values", () => {
    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    expect(screen.getByTestId("passkey-name")).toHaveTextContent("MacBook Touch ID");
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

  it("deletes the enrolled passkey through the API client", async () => {
    mockDeletePasskey.mockResolvedValue();
    const user = userEvent.setup();

    render(<Passkeys factors={[PASSKEY_FACTOR]} onChanged={onChanged} />);
    await user.click(screen.getByTestId("passkey-delete-button"));

    await waitFor(() => {
      expect(mockDeletePasskey).toHaveBeenCalledOnce();
    });
    await waitFor(() => {
      expect(onChanged).toHaveBeenCalledOnce();
    });
  });
});
