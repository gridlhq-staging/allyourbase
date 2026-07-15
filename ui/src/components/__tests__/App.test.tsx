import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { MockApiError } from "../../test-utils";

vi.mock("../../api", () => ({
  getAdminStatus: vi.fn(),
  getSchema: vi.fn(),
  clearAuthToken: vi.fn(),
  clearToken: vi.fn(),
  ApiError: MockApiError,
}));

vi.mock("../../api_capabilities", () => ({
  getAdminCapabilities: vi.fn(),
}));

// Mock child components to isolate App logic.
vi.mock("../Login", () => ({
  Login: ({ onSuccess }: { onSuccess: () => void }) => (
    <div data-testid="login">
      <button onClick={onSuccess}>mock-login</button>
    </div>
  ),
}));

vi.mock("../OAuthConsent", () => ({
  OAuthConsent: () => <div data-testid="oauth-consent" />,
}));

vi.mock("../Layout", async () => {
  const { useCapability } =
    await vi.importActual<typeof import("../../capabilities")>("../../capabilities");

  return {
    Layout: ({
    onLogout,
    onRefresh,
  }: {
    onLogout: () => void;
    onRefresh: () => void;
  }) => {
    const capabilities = useCapability();
    return (
      <div data-testid="layout" data-capability-state={capabilities.state.kind}>
        <button onClick={onLogout}>mock-logout</button>
        <button onClick={onRefresh}>mock-refresh</button>
      </div>
    );
  },
  };
});

import { getAdminStatus, getSchema, clearAuthToken, clearToken } from "../../api";
import { getAdminCapabilities } from "../../api_capabilities";
import { App } from "../../App";

const mockGetAdminStatus = vi.mocked(getAdminStatus);
const mockGetSchema = vi.mocked(getSchema);
const mockClearAuthToken = vi.mocked(clearAuthToken);
const mockClearToken = vi.mocked(clearToken);
const mockGetAdminCapabilities = vi.mocked(getAdminCapabilities);

const fakeSchema = {
  tables: {},
  schemas: ["public"],
  builtAt: "2024-01-01T00:00:00Z",
};

describe("App", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    window.history.replaceState(null, "", "/admin/");
    mockGetAdminCapabilities.mockResolvedValue({ kind: "unknown" });
  });

  it("short-circuits OAuth consent before dashboard boot", () => {
    window.history.replaceState(null, "", "/oauth/authorize?client_id=test");
    render(<App />);
    expect(screen.getByTestId("oauth-consent")).toBeInTheDocument();
    expect(mockGetAdminStatus).not.toHaveBeenCalled();
  });

  it("shows loading state initially", () => {
    // Keep promises pending so we stay in loading.
    mockGetAdminStatus.mockReturnValue(new Promise(() => {}));
    render(<App />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("shows login when admin auth required and no token", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });
    expect(mockGetSchema).not.toHaveBeenCalled();
  });

  it("loads schema when no auth required", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });
  });

  it("loads schema when auth required but token exists", async () => {
    localStorage.setItem("ayb_admin_token", "tok");
    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });
  });

  it("waits for capabilities before first authenticated layout render", async () => {
    localStorage.setItem("ayb_admin_token", "tok");
    let resolveCapabilities: (value: Awaited<ReturnType<typeof getAdminCapabilities>>) => void;
    const capabilitiesPromise = new Promise<Awaited<ReturnType<typeof getAdminCapabilities>>>((resolve) => {
      resolveCapabilities = resolve;
    });
    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    mockGetAdminCapabilities.mockReturnValueOnce(capabilitiesPromise);
    mockGetSchema.mockResolvedValueOnce(fakeSchema);

    render(<App />);

    await waitFor(() => expect(mockGetAdminCapabilities).toHaveBeenCalledOnce());
    expect(mockGetSchema).not.toHaveBeenCalled();
    expect(screen.queryByTestId("layout")).not.toBeInTheDocument();

    resolveCapabilities!({ kind: "unknown" });

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });
  });

  it("passes unknown capabilities through the passwordless capability-401 path", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetAdminCapabilities.mockResolvedValueOnce({ kind: "unknown" });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);

    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toHaveAttribute("data-capability-state", "unknown");
    });
    expect(screen.queryByTestId("login")).not.toBeInTheDocument();
  });

  it("keeps capability failures out of the admin-auth failure path", async () => {
    const unauthorizedListener = vi.fn();
    window.addEventListener("ayb:unauthorized", unauthorizedListener);
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetAdminCapabilities.mockResolvedValueOnce({ kind: "unknown" });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);

    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });

    expect(mockClearAuthToken).not.toHaveBeenCalled();
    expect(mockClearToken).not.toHaveBeenCalled();
    expect(screen.queryByTestId("login")).not.toBeInTheDocument();
    expect(unauthorizedListener).not.toHaveBeenCalled();
    window.removeEventListener("ayb:unauthorized", unauthorizedListener);
  });

  it("shows login on 401 from getSchema", async () => {
    localStorage.setItem("ayb_admin_token", "expired");
    const { ApiError } = await import("../../api");
    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    mockGetSchema.mockRejectedValueOnce(new ApiError(401, "unauthorized"));
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });
    expect(mockClearAuthToken).toHaveBeenCalled();
    expect(mockClearToken).toHaveBeenCalled();
  });

  it("shows error state on non-401 failure", async () => {
    mockGetAdminStatus.mockRejectedValueOnce(new Error("connection refused"));
    render(<App />);

    await waitFor(() => {
      expect(screen.getByText("Connection Error")).toBeInTheDocument();
      expect(screen.getByText("connection refused")).toBeInTheDocument();
    });
  });

  it("retry button re-triggers boot", async () => {
    mockGetAdminStatus.mockRejectedValueOnce(new Error("fail"));
    render(<App />);

    await waitFor(() => {
      expect(screen.getByText("Connection Error")).toBeInTheDocument();
    });

    // Now succeed on retry.
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });
  });

  it("login success triggers boot and shows layout", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });

    // After login, boot runs again.
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "mock-login" }));

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });
  });

  it("logout clears token and shows login", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "mock-logout" }));

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });
    expect(mockClearAuthToken).toHaveBeenCalled();
    expect(mockClearToken).toHaveBeenCalled();
  });

  it("refresh reloads schema without leaving layout", async () => {
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });

    const updatedSchema = { ...fakeSchema, builtAt: "2024-06-01T00:00:00Z" };
    mockGetSchema.mockResolvedValueOnce(updatedSchema);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "mock-refresh" }));

    await waitFor(() => {
      expect(mockGetSchema).toHaveBeenCalledTimes(2);
    });
    // Still on layout, not login.
    expect(screen.getByTestId("layout")).toBeInTheDocument();
  });

  it("redirects to return_to URL after login", async () => {
    const originalLocation = window.location;
    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        pathname: "/",
        search: "?return_to=%2Foauth%2Fauthorize%3Fclient_id%3Dtest%26state%3Dxyz",
        href:
          "http://localhost/?return_to=%2Foauth%2Fauthorize%3Fclient_id%3Dtest%26state%3Dxyz",
        assign: assignMock,
      },
      writable: true,
    });

    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "mock-login" }));

    await waitFor(() => {
      expect(assignMock).toHaveBeenCalledTimes(1);
    });

    const redirectTarget = assignMock.mock.calls[0][0] as string;
    expect(redirectTarget.startsWith("/oauth/authorize")).toBe(true);
    expect(redirectTarget).toContain("client_id=test");
    expect(redirectTarget).toContain("state=xyz");

    // Restore
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
    });
  });

  it("does not redirect to protocol-relative return_to URLs (open redirect prevention)", async () => {
    const originalLocation = window.location;
    const assignMock = vi.fn();
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        pathname: "/",
        search: "?return_to=%2F%2Fevil.com%2Fsteal",
        href: "http://localhost/?return_to=%2F%2Fevil.com%2Fsteal",
        assign: assignMock,
      },
      writable: true,
    });

    mockGetAdminStatus.mockResolvedValueOnce({ auth: true });
    render(<App />);

    await waitFor(() => {
      expect(screen.getByTestId("login")).toBeInTheDocument();
    });

    // After login, boot runs normally — no external redirect.
    mockGetAdminStatus.mockResolvedValueOnce({ auth: false });
    mockGetSchema.mockResolvedValueOnce(fakeSchema);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "mock-login" }));

    await waitFor(() => {
      expect(screen.getByTestId("layout")).toBeInTheDocument();
    });

    // Should NOT have redirected to the external URL.
    expect(assignMock).not.toHaveBeenCalled();

    // Restore
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
    });
  });
});
