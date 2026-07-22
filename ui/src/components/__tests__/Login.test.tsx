import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MockApiError } from "../../test-utils";
import { docsUrl } from "../../lib/docs_url";

vi.mock("../../api", () => ({
  adminLogin: vi.fn(),
  ApiError: MockApiError,
}));

import { Login } from "../Login";
import { adminLogin } from "../../api";
const mockAdminLogin = vi.mocked(adminLogin);

function getPasswordInput(): HTMLInputElement {
  return document.querySelector('input[type="password"]') as HTMLInputElement;
}

describe("Login", () => {
  const onSuccess = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders the login form", () => {
    render(<Login onSuccess={onSuccess} />);

    expect(screen.getByText("👾 Allyourbase")).toBeInTheDocument();
    expect(screen.getByText("Password")).toBeInTheDocument();
    expect(getPasswordInput()).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeInTheDocument();
  });

  it("calls adminLogin and onSuccess on successful submit", async () => {
    mockAdminLogin.mockResolvedValueOnce("fake-token");
    const user = userEvent.setup();

    render(<Login onSuccess={onSuccess} />);

    await user.type(getPasswordInput(), "mypassword");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      expect(mockAdminLogin).toHaveBeenCalledWith("mypassword");
      expect(onSuccess).toHaveBeenCalled();
    });
  });

  it("shows RF-016 unauthorized login with form recovery guidance", async () => {
    const { ApiError } = await import("../../api");
    mockAdminLogin.mockRejectedValueOnce(new ApiError(401, "invalid password"));
    const user = userEvent.setup();

    render(<Login onSuccess={onSuccess} />);

    await user.type(getPasswordInput(), "wrong");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      expect(screen.getByText("invalid password")).toBeInTheDocument();
    });
    expect(getPasswordInput()).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sign in" })).toBeEnabled();
    expect(screen.getByRole("link", { name: "View guide" })).toHaveAttribute(
      "href",
      docsUrl("/guide/authentication"),
    );
    expect(onSuccess).not.toHaveBeenCalled();
  });

  it("shows generic error for non-API errors", async () => {
    mockAdminLogin.mockRejectedValueOnce(new Error("Network error"));
    const user = userEvent.setup();

    render(<Login onSuccess={onSuccess} />);

    await user.type(getPasswordInput(), "test");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      expect(
        screen.getByText("Failed to connect to server"),
      ).toBeInTheDocument();
    });
  });

  it("disables button while loading", async () => {
    // Make login hang.
    mockAdminLogin.mockImplementation(
      () => new Promise(() => {}),
    );
    const user = userEvent.setup();

    render(<Login onSuccess={onSuccess} />);

    await user.type(getPasswordInput(), "test");
    await user.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Signing in..." })).toBeDisabled();
    });
  });

  it("requires password field (HTML validation)", () => {
    render(<Login onSuccess={onSuccess} />);
    expect(getPasswordInput()).toBeRequired();
  });
});
