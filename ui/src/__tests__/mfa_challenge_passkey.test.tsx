import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MFAChallenge } from "../components/MFAChallenge";
import type { AuthTokens } from "../types";
import {
  beginPasskeyChallenge,
  challengeEmailMFA,
  challengeSMSMFA,
  challengeTOTP,
  getMFAFactors,
  verifyBackupCode,
  verifyEmailMFA,
  verifyPasskeyChallenge,
  verifySMSMFA,
  verifyTOTP,
} from "../api";
import { createPasskeyAssertion } from "../webauthn";

vi.mock("../api", () => ({
  getMFAFactors: vi.fn(),
  challengeTOTP: vi.fn(),
  verifyTOTP: vi.fn(),
  challengeSMSMFA: vi.fn(),
  verifySMSMFA: vi.fn(),
  challengeEmailMFA: vi.fn(),
  verifyEmailMFA: vi.fn(),
  verifyBackupCode: vi.fn(),
  beginPasskeyChallenge: vi.fn(),
  verifyPasskeyChallenge: vi.fn(),
}));

vi.mock("../webauthn", () => ({
  createPasskeyAssertion: vi.fn(),
}));

const mockGetMFAFactors = vi.mocked(getMFAFactors);
const mockChallengeTOTP = vi.mocked(challengeTOTP);
const mockVerifyTOTP = vi.mocked(verifyTOTP);
const mockChallengeSMSMFA = vi.mocked(challengeSMSMFA);
const mockVerifySMSMFA = vi.mocked(verifySMSMFA);
const mockChallengeEmailMFA = vi.mocked(challengeEmailMFA);
const mockVerifyEmailMFA = vi.mocked(verifyEmailMFA);
const mockVerifyBackupCode = vi.mocked(verifyBackupCode);
const mockBeginPasskeyChallenge = vi.mocked(beginPasskeyChallenge);
const mockVerifyPasskeyChallenge = vi.mocked(verifyPasskeyChallenge);
const mockCreatePasskeyAssertion = vi.mocked(createPasskeyAssertion);

const upgradedTokens: AuthTokens = {
  token: "aal2-token",
  refreshToken: "refresh-token",
  user: {
    id: "user-1",
    email: "user@example.com",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
};

describe("MFAChallenge passkey branch", () => {
  const onVerified = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    mockChallengeTOTP.mockResolvedValue({ challenge_id: "unused-totp" });
    mockVerifyTOTP.mockResolvedValue(upgradedTokens);
    mockChallengeSMSMFA.mockResolvedValue({ message: "unused-sms" });
    mockVerifySMSMFA.mockResolvedValue(upgradedTokens);
    mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "unused-email" });
    mockVerifyEmailMFA.mockResolvedValue(upgradedTokens);
    mockVerifyBackupCode.mockResolvedValue(upgradedTokens);
  });

  it("requests challenge, creates assertion, verifies passkey, and forwards upgraded tokens", async () => {
    const user = userEvent.setup();
    mockGetMFAFactors.mockResolvedValue({
      factors: [{ id: "factor-passkey", method: "webauthn", label: "Device Passkey" }],
    });
    mockBeginPasskeyChallenge.mockResolvedValue({
      challenge_id: "passkey-challenge-1",
      options: { challenge: "Y2hhbGxlbmdl", allowCredentials: [] },
    });
    mockCreatePasskeyAssertion.mockResolvedValue({ id: "assertion-1" });
    mockVerifyPasskeyChallenge.mockResolvedValue(upgradedTokens);

    render(<MFAChallenge onVerified={onVerified} />);

    await waitFor(() => {
      expect(screen.getByTestId("passkey-verify-button")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("passkey-verify-button"));

    await waitFor(() => {
      expect(mockCreatePasskeyAssertion).toHaveBeenCalledWith({ challenge: "Y2hhbGxlbmdl", allowCredentials: [] });
    });
    await waitFor(() => {
      expect(mockVerifyPasskeyChallenge).toHaveBeenCalledWith("passkey-challenge-1", { id: "assertion-1" });
    });
    await waitFor(() => {
      expect(onVerified).toHaveBeenCalledWith(upgradedTokens);
    });
  });

  it("surfaces actionable passkey verification errors", async () => {
    const user = userEvent.setup();
    mockGetMFAFactors.mockResolvedValue({
      factors: [{ id: "factor-passkey", method: "webauthn", label: "Device Passkey" }],
    });
    mockBeginPasskeyChallenge.mockResolvedValue({
      challenge_id: "passkey-challenge-2",
      options: { challenge: "Y2hhbGxlbmdl" },
    });
    mockCreatePasskeyAssertion.mockRejectedValue(new Error("Passkey prompt was cancelled"));

    render(<MFAChallenge onVerified={onVerified} />);

    await waitFor(() => {
      expect(screen.getByTestId("passkey-verify-button")).toBeInTheDocument();
    });

    await user.click(screen.getByTestId("passkey-verify-button"));

    expect(await screen.findByText(/passkey prompt was cancelled/i)).toBeInTheDocument();
    expect(onVerified).not.toHaveBeenCalled();
  });
});
