import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders, expectWcagContrastToken } from "../../test-utils";
import { AuthHooks } from "../AuthHooks";

vi.mock("../../api_auth_hooks", () => ({
  getAuthHooks: vi.fn(),
}));

import * as api from "../../api_auth_hooks";

const mockHooks = {
  before_sign_up: "validate_email",
  after_sign_up: "provision_workspace",
  custom_access_token: "",
  before_password_reset: "",
  send_email: "custom_mailer",
  send_sms: "",
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.getAuthHooks as ReturnType<typeof vi.fn>).mockResolvedValue(mockHooks);
});

describe("AuthHooks", () => {
  it("displays 6 hook slots with function name or not configured", async () => {
    renderWithProviders(<AuthHooks />);
    await waitFor(() => {
      expect(screen.getByText("validate_email")).toBeInTheDocument();
    });
    expect(screen.getByText("provision_workspace")).toBeInTheDocument();
    expect(screen.getByText("custom_mailer")).toBeInTheDocument();
    // 3 empty slots should show "Not configured"
    const notConfigured = screen.getAllByText("Not configured");
    expect(notConfigured.length).toBe(3);
  });

  it("unconfigured hook text uses WCAG AA compliant contrast token", async () => {
    renderWithProviders(<AuthHooks />);
    await waitFor(() => {
      expect(screen.getByText("validate_email")).toBeInTheDocument();
    });

    const className = screen.getAllByText("Not configured")[0].className;
    expectWcagContrastToken(className);
  });

  it("shows error state on fetch failure", async () => {
    (api.getAuthHooks as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("Auth error"));
    renderWithProviders(<AuthHooks />);
    await waitFor(() => {
      expect(screen.getByText("Auth error")).toBeInTheDocument();
    });
  });

  it("recovers from an auth hooks load failure through an error-scoped Retry that refetches", async () => {
    const user = userEvent.setup();
    // First load rejects; the beforeEach default (mockHooks) satisfies the retry.
    (api.getAuthHooks as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("Auth error"));
    renderWithProviders(<AuthHooks />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Auth error");
    // Panel shell (heading) stays mounted alongside the error.
    expect(screen.getByRole("heading", { name: "Auth Hooks" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(screen.getByText("validate_email")).toBeInTheDocument());
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
