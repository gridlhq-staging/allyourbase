import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiKeys } from "../ApiKeys";
import { listApiKeys, createApiKey, revokeApiKey, listUsers } from "../../api";
import type {
  APIKeyResponse,
  APIKeyListResponse,
  APIKeyCreateResponse,
  UserListResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listApiKeys: vi.fn(),
  createApiKey: vi.fn(),
  revokeApiKey: vi.fn(),
  listUsers: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListApiKeys = vi.mocked(listApiKeys);
const mockCreateApiKey = vi.mocked(createApiKey);
const mockRevokeApiKey = vi.mocked(revokeApiKey);
const mockListUsers = vi.mocked(listUsers);

function makeKey(overrides: Partial<APIKeyResponse> = {}): APIKeyResponse {
  return {
    id: "k1",
    userId: "u1",
    name: "CI/CD",
    keyPrefix: "ayb_abc12345",
    scope: "*",
    allowedTables: null,
    lastUsedAt: null,
    expiresAt: null,
    createdAt: "2026-02-10T12:00:00Z",
    revokedAt: null,
    ...overrides,
  };
}

function makeResponse(
  keys: APIKeyResponse[] = [],
  overrides: Partial<APIKeyListResponse> = {},
): APIKeyListResponse {
  return {
    items: keys,
    page: 1,
    perPage: 20,
    totalItems: keys.length,
    totalPages: keys.length > 0 ? 1 : 0,
    ...overrides,
  };
}

function makeUserResponse(): UserListResponse {
  return {
    items: [
      {
        id: "u1",
        email: "alice@example.com",
        emailVerified: true,
        createdAt: "2026-02-10T12:00:00Z",
        updatedAt: "2026-02-10T12:00:00Z",
      },
    ],
    page: 1,
    perPage: 100,
    totalItems: 1,
    totalPages: 1,
  };
}

describe("ApiKeys", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListUsers.mockResolvedValue(makeUserResponse());
  });

  it("shows loading state", () => {
    mockListApiKeys.mockReturnValue(new Promise(() => {}));
    render(<ApiKeys />);
    expect(screen.getByText("Loading API keys...")).toBeInTheDocument();
  });

  it("renders API keys list", async () => {
    const keys = [
      makeKey({ id: "k1", name: "CI/CD" }),
      makeKey({ id: "k2", name: "Backend Service", keyPrefix: "ayb_def67890" }),
    ];
    mockListApiKeys.mockResolvedValueOnce(makeResponse(keys));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
      expect(screen.getByText("Backend Service")).toBeInTheDocument();
    });
  });

  it("shows empty state when no keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });
  });

  it("shows error state with retry", async () => {
    mockListApiKeys.mockRejectedValueOnce(new Error("connection refused"));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("connection refused")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("shows total key count", async () => {
    const keys = [makeKey()];
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse(keys, { totalItems: 1 }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("1 key")).toBeInTheDocument();
    });
  });

  it("shows plural key count", async () => {
    const keys = [
      makeKey({ id: "k1", name: "CI/CD" }),
      makeKey({ id: "k2", name: "Backend" }),
    ];
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse(keys, { totalItems: 2 }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("2 keys")).toBeInTheDocument();
    });
  });

  it("shows key prefix with ellipsis", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ keyPrefix: "ayb_abc12345" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("ayb_abc12345...")).toBeInTheDocument();
    });
  });

  it("shows Active status for non-revoked keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Active")).toBeInTheDocument();
    });
  });

  it("shows Revoked status for revoked keys", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: "2026-02-10T13:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });
  });

  it("shows Never for keys that haven't been used", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ lastUsedAt: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Never")).toBeInTheDocument();
    });
  });

  it("create button opens create modal", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    expect(screen.getByText("Create API Key")).toBeInTheDocument();
    expect(screen.getByLabelText("Key name")).toBeInTheDocument();
  });

  it("create flow calls createApiKey and shows key", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    const createdResponse: APIKeyCreateResponse = {
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Deploy Key" }),
    };
    mockCreateApiKey.mockResolvedValueOnce(createdResponse);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));

    // Fill in the form
    await user.type(screen.getByLabelText("Key name"), "Deploy Key");
    // Select user from dropdown
    await user.selectOptions(screen.getByLabelText("User"), "u1");

    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Deploy Key",
        scope: "*",
      });
    });

    // Should show the created key
    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(
        screen.getByText(
          "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
        ),
      ).toBeInTheDocument();
      expect(
        screen.getByText("Copy this key now. It will not be shown again."),
      ).toBeInTheDocument();
    });
  });

  it("revoke button opens confirmation dialog", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    expect(screen.getByText("Revoke API Key")).toBeInTheDocument();
    expect(
      screen.getByText(
        /This will permanently revoke the API key/,
      ),
    ).toBeInTheDocument();
  });

  it("confirming revoke calls revokeApiKey and refreshes", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([makeKey()]));
    mockRevokeApiKey.mockResolvedValueOnce(undefined);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    const dialog = screen
      .getByText("Revoke API Key")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Revoke" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeApiKey).toHaveBeenCalledWith("k1");
    });
  });

  it("cancel on revoke dialog closes it", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));
    expect(screen.getByText("Revoke API Key")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Revoke API Key")).not.toBeInTheDocument();
  });

  it("shows user email for known users", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ userId: "u1" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("alice@example.com")).toBeInTheDocument();
    });
  });

  it("revoked keys don't show revoke button", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ revokedAt: "2026-02-10T13:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Revoked")).toBeInTheDocument();
    });

    expect(screen.queryByTitle("Revoke key")).not.toBeInTheDocument();
  });

  it("displays page info for multi-page results", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey()], {
        totalItems: 45,
        totalPages: 3,
        page: 1,
      }),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("45 keys")).toBeInTheDocument();
      expect(screen.getByText("1 / 3")).toBeInTheDocument();
    });
  });

  it("retry button refetches keys after error", async () => {
    mockListApiKeys.mockRejectedValueOnce(new Error("network down"));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("network down")).toBeInTheDocument();
    });

    // Retry should fetch again
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    const user = userEvent.setup();
    await user.click(screen.getByText("Retry"));

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });
    // Should have been called twice: initial + retry
    expect(mockListApiKeys).toHaveBeenCalledTimes(2);
  });

  it("create button disabled when name is empty", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    // Select user but leave name empty
    await user.selectOptions(screen.getByLabelText("User"), "u1");

    const createBtn = screen.getByRole("button", { name: "Create" });
    expect(createBtn).toBeDisabled();
  });

  it("create button disabled when user is not selected", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    // Type name but don't select user
    await user.type(screen.getByLabelText("Key name"), "Test Key");

    const createBtn = screen.getByRole("button", { name: "Create" });
    expect(createBtn).toBeDisabled();
  });

  it("cancel on create modal closes it", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));
    expect(screen.getByText("Create API Key")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(screen.queryByText("Create API Key")).not.toBeInTheDocument();
  });

  it("done button on created modal closes it", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Test Key" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
    });

    await user.click(screen.getByText("Done"));
    expect(screen.queryByText("API Key Created")).not.toBeInTheDocument();
  });

  it("create refreshes key list after success", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_fullkeyshownonce1234567890abcdef0123456789ab",
      apiKey: makeKey({ id: "k-new", name: "Test Key" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
    });

    // listApiKeys should have been called twice: initial load + refresh after create
    expect(mockListApiKeys).toHaveBeenCalledTimes(2);
  });

  it("revoke refreshes key list after success", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([makeKey()]));
    mockRevokeApiKey.mockResolvedValueOnce(undefined);
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByTitle("Revoke key"));

    const dialog = screen
      .getByText("Revoke API Key")
      .closest("div.fixed")! as HTMLElement;
    const confirmBtn = within(dialog).getByRole("button", { name: "Revoke" });
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(mockRevokeApiKey).toHaveBeenCalledWith("k1");
    });
    // listApiKeys should have been called multiple times (initial + refresh after revoke)
    expect(mockListApiKeys.mock.calls.length).toBeGreaterThanOrEqual(2);
  });

  it("shows last used date when available", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ lastUsedAt: "2026-02-09T08:00:00Z" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      // "Never" should not appear since lastUsedAt is set
      expect(screen.queryByText("Never")).not.toBeInTheDocument();
      // The actual formatted date should appear
      const expectedDate = new Date("2026-02-09T08:00:00Z").toLocaleDateString();
      expect(screen.getByText(expectedDate)).toBeInTheDocument();
    });
  });

  it("shows user ID when email not available", async () => {
    mockListUsers.mockResolvedValueOnce({
      items: [],
      page: 1,
      perPage: 100,
      totalItems: 0,
      totalPages: 0,
    });
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ userId: "unknown-user-id" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("unknown-user-id")).toBeInTheDocument();
    });
  });

  it("shows full access badge for * scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "*" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("full access")).toBeInTheDocument();
    });
  });

  it("shows readonly badge for readonly scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "readonly" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("readonly")).toBeInTheDocument();
    });
  });

  it("shows readwrite badge for readwrite scope", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ scope: "readwrite" })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("readwrite")).toBeInTheDocument();
    });
  });

  it("shows allowed tables when restricted", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ allowedTables: ["posts", "comments"] })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("posts, comments")).toBeInTheDocument();
    });
  });

  it("does not show tables when allowedTables is null", async () => {
    mockListApiKeys.mockResolvedValueOnce(
      makeResponse([makeKey({ allowedTables: null })]),
    );
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("full access")).toBeInTheDocument();
    });
    expect(screen.queryByText("posts")).not.toBeInTheDocument();
  });

  it("scope column header is visible", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("Scope")).toBeInTheDocument();
    });
  });

  it("create form has scope selector defaulting to full access", async () => {
    mockListApiKeys.mockResolvedValueOnce(makeResponse([makeKey()]));
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("CI/CD")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create Key"));

    const scopeSelect = screen.getByLabelText("Scope") as HTMLSelectElement;
    expect(scopeSelect.value).toBe("*");
  });

  it("create form sends readonly scope when selected", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Readonly Key", scope: "readonly" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Readonly Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readonly");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Readonly Key",
        scope: "readonly",
      });
    });
  });

  it("create form sends allowed tables when specified", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Limited Key", scope: "readwrite", allowedTables: ["posts"] }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Limited Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readwrite");
    await user.type(screen.getByLabelText("Allowed tables"), "posts");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(mockCreateApiKey).toHaveBeenCalledWith({
        userId: "u1",
        name: "Limited Key",
        scope: "readwrite",
        allowedTables: ["posts"],
      });
    });
  });

  it("created modal shows scope info", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({ id: "k-new", name: "Test Key", scope: "readonly" }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Test Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.selectOptions(screen.getByLabelText("Scope"), "readonly");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(screen.getByText(/readonly/)).toBeInTheDocument();
    });
  });

  it("created modal shows allowed tables when restricted", async () => {
    mockListApiKeys.mockResolvedValue(makeResponse([]));
    mockCreateApiKey.mockResolvedValueOnce({
      key: "ayb_testkey",
      apiKey: makeKey({
        id: "k-new",
        name: "Table Key",
        scope: "readwrite",
        allowedTables: ["orders", "products"],
      }),
    });
    render(<ApiKeys />);

    await waitFor(() => {
      expect(screen.getByText("No API keys created yet")).toBeInTheDocument();
    });

    const user = userEvent.setup();
    await user.click(screen.getByText("Create your first API key"));
    await user.type(screen.getByLabelText("Key name"), "Table Key");
    await user.selectOptions(screen.getByLabelText("User"), "u1");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => {
      expect(screen.getByText("API Key Created")).toBeInTheDocument();
      expect(screen.getByText("orders, products")).toBeInTheDocument();
    });
  });
});
