import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Layout } from "../Layout";
import { ThemeProvider } from "../ThemeProvider";
import type { SchemaCache } from "../../types";
import type { ReactElement } from "react";

function renderWithTheme(ui: ReactElement) {
  return render(<ThemeProvider>{ui}</ThemeProvider>);
}

// Mock child components to isolate Layout logic.
vi.mock("../TableBrowser", () => ({
  TableBrowser: ({ table }: { table: { name: string } }) => (
    <div data-testid="table-browser">{table.name}</div>
  ),
}));

vi.mock("../SchemaView", () => ({
  SchemaView: ({ table }: { table: { name: string } }) => (
    <div data-testid="schema-view">{table.name}</div>
  ),
}));

vi.mock("../SqlEditor", () => ({
  SqlEditor: () => <div data-testid="sql-editor" />,
}));

vi.mock("../SearchSettingsEditor", () => ({
  SearchSettingsEditor: ({ selected }: { selected: { name: string } }) => (
    <div data-testid="search-settings-view">{selected.name}</div>
  ),
}));

vi.mock("../Webhooks", () => ({
  Webhooks: () => <div data-testid="webhooks-view" />,
}));

vi.mock("../StorageBrowser", () => ({
  StorageBrowser: () => <div data-testid="storage-view" />,
}));

vi.mock("../Users", () => ({
  Users: () => <div data-testid="users-view" />,
}));

vi.mock("../FunctionBrowser", () => ({
  FunctionBrowser: () => <div data-testid="functions-view" />,
}));

vi.mock("../SMSHealth", () => ({
  SMSHealth: () => <div data-testid="sms-health-view" />,
}));

vi.mock("../SMSMessages", () => ({
  SMSMessages: () => <div data-testid="sms-messages-view" />,
}));

vi.mock("../Jobs", () => ({
  Jobs: () => <div data-testid="jobs-view" />,
}));

vi.mock("../Schedules", () => ({
  Schedules: () => <div data-testid="schedules-view" />,
}));

vi.mock("../EmailTemplates", () => ({
  EmailTemplates: () => <div data-testid="email-templates-view" />,
}));

vi.mock("../PushNotifications", () => ({
  PushNotifications: () => <div data-testid="push-view" />,
}));

vi.mock("../MFAEnrollment", () => ({
  MFAEnrollment: () => <div data-testid="mfa-enrollment-view" />,
}));

vi.mock("../AccountLinking", () => ({
  AccountLinking: () => <div data-testid="account-linking-view" />,
}));

vi.mock("../RealtimeInspector", () => ({
  RealtimeInspector: () => <div data-testid="realtime-inspector-view" />,
}));

vi.mock("../SecurityAdvisor", () => ({
  SecurityAdvisor: () => <div data-testid="security-advisor-view" />,
}));

vi.mock("../PerformanceAdvisor", () => ({
  PerformanceAdvisor: () => <div data-testid="performance-advisor-view" />,
}));

vi.mock("../AdminLogs", () => ({
  AdminLogs: () => <div data-testid="admin-logs-view" />,
}));

vi.mock("../UsageMetering", () => ({
  UsageMetering: () => <div data-testid="usage-metering-view" />,
}));

vi.mock("../MFAChallenge", () => ({
  MFAChallenge: () => <div data-testid="mfa-challenge-view" />,
}));

vi.mock("../Tenants", () => ({
  Tenants: () => <div data-testid="tenants-view" />,
}));

vi.mock("../Organizations", () => ({
  Organizations: () => <div data-testid="organizations-view" />,
}));

function makeSchema(
  tables: Record<string, { schema: string; name: string; kind: string }> = {},
): SchemaCache {
  const full: SchemaCache = {
    schemas: ["public"],
    builtAt: "2024-01-01T00:00:00Z",
    tables: {},
  };
  for (const [key, t] of Object.entries(tables)) {
    full.tables[key] = {
      ...t,
      columns: [],
      primaryKey: [],
    };
  }
  return full;
}

const twoTableSchema = makeSchema({
  "public.posts": { schema: "public", name: "posts", kind: "table" },
  "public.users": { schema: "public", name: "users", kind: "table" },
});

describe("Layout", () => {
  const onLogout = vi.fn();
  const onRefresh = vi.fn();

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.clearAllMocks();
    localStorage.clear();
    document.head.innerHTML = '<meta name="ayb-admin-base" content="/admin/">';
    window.history.replaceState(null, "", "/admin/");
    document.documentElement.classList.remove("dark");
  });

  it("restores an exact table from a deep link", () => {
    window.history.replaceState(null, "", "/admin/tables/public/users");
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByTestId("table-browser")).toHaveTextContent("users");
  });

  it("pushes one pathname change and preserves query and hash", async () => {
    window.history.replaceState(null, "", "/admin/?perfRange=24h#slow");
    const pushState = vi.spyOn(window.history, "pushState");
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );

    await userEvent.setup().click(screen.getByText("Webhooks"));

    expect(pushState).toHaveBeenCalledOnce();
    expect(window.location.pathname).toBe("/admin/screens/webhooks");
    expect(window.location.search).toBe("?perfRange=24h");
    expect(window.location.hash).toBe("#slow");
    expect(screen.getByTestId("webhooks-view")).toBeInTheDocument();
  });

  it("applies popstate without pushing a history entry", async () => {
    const pushState = vi.spyOn(window.history, "pushState");
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    act(() => {
      window.history.replaceState(null, "", "/admin/screens/sql-editor");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });

    await waitFor(() => expect(screen.getByTestId("sql-editor")).toBeInTheDocument());
    expect(pushState).not.toHaveBeenCalled();
  });

  it("canonicalizes a valid deep link with replaceState", async () => {
    window.history.replaceState(null, "", "/admin/screens/sql%2Deditor/?filter=slow#query");
    const replaceState = vi.spyOn(window.history, "replaceState");
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );

    await waitFor(() => expect(screen.getByTestId("sql-editor")).toBeInTheDocument());
    expect(replaceState).toHaveBeenCalledOnce();
    expect(`${window.location.pathname}${window.location.search}${window.location.hash}`).toBe(
      "/admin/screens/sql-editor?filter=slow#query",
    );
  });

  it("returns from a route failure with one ordinary push", async () => {
    window.history.replaceState(null, "", "/admin/screens/missing?filter=slow#query");
    const pushState = vi.spyOn(window.history, "pushState");
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );

    await userEvent.setup().click(screen.getByRole("button", { name: "Return to console" }));
    expect(pushState).toHaveBeenCalledOnce();
    expect(`${window.location.pathname}${window.location.search}${window.location.hash}`).toBe(
      "/admin/?filter=slow#query",
    );
    expect(screen.getByTestId("table-browser")).toHaveTextContent("posts");
  });

  it("re-resolves a missing table after schema changes without pushing", async () => {
    window.history.replaceState(null, "", "/admin/tables/public/users");
    const pushState = vi.spyOn(window.history, "pushState");
    const { rerender } = renderWithTheme(
      <Layout schema={makeSchema()} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Table not found")).toBeInTheDocument();

    rerender(
      <ThemeProvider>
        <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />
      </ThemeProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("table-browser")).toHaveTextContent("users"));
    expect(pushState).not.toHaveBeenCalled();
  });

  it("renders sidebar with table names", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    // "posts" appears in both sidebar and header, so use getAllByText.
    expect(screen.getAllByText("posts").length).toBeGreaterThanOrEqual(1);
    expect(screen.getAllByText("users").length).toBeGreaterThanOrEqual(1);
  });

  it("selects first table by default and shows TableBrowser", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
    const schemaTabClasses = screen
      .getByRole("button", { name: "Schema" })
      .className.split(" ");
    const sqlTabClasses = screen
      .getByRole("button", { name: "SQL" })
      .className.split(" ");
    expect(schemaTabClasses).toContain("text-gray-600");
    expect(schemaTabClasses).not.toContain("text-gray-500");
    expect(sqlTabClasses).toContain("text-gray-600");
    expect(sqlTabClasses).not.toContain("text-gray-500");
  });

  it("shows empty state when no tables", () => {
    renderWithTheme(
      <Layout schema={makeSchema()} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("No tables yet")).toBeInTheDocument();
    const sidebarHelperClasses = screen
      .getByText("Create your first table to get started.")
      .className.split(" ");
    expect(sidebarHelperClasses).toContain("text-gray-500");
    expect(sidebarHelperClasses).not.toContain("text-gray-400");
    expect(screen.getByText("Select a table from the sidebar")).toBeInTheDocument();
    const emptyStateClasses = screen
      .getByText("Select a table from the sidebar")
      .parentElement?.className.split(" ");
    expect(emptyStateClasses).toContain("text-gray-500");
    expect(emptyStateClasses).not.toContain("text-gray-400");
    const helperTextClasses = screen
      .getByText("Use SQL Editor from the sidebar to create one.")
      .className.split(" ");
    expect(helperTextClasses).toContain("text-gray-600");
    expect(helperTextClasses).not.toContain("text-gray-400");
  });

  it("keeps a single Open SQL Editor CTA owner in empty-schema surfaces", () => {
    renderWithTheme(
      <Layout schema={makeSchema()} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getAllByRole("button", { name: /open sql editor/i })).toHaveLength(1);
    expect(
      screen.getByText("Use SQL Editor from the sidebar to create one."),
    ).toBeInTheDocument();
  });

  it("clicking a table selects it and switches to data view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("users"));
    expect(screen.getByTestId("table-browser")).toHaveTextContent("users");
  });

  it("switches to schema view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Schema"));
    expect(screen.getByTestId("schema-view")).toBeInTheDocument();
  });

  it("switches to SQL view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SQL"));
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();
  });

  it("switches to Search Settings view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Search Settings"));
    expect(screen.getByTestId("search-settings-view")).toHaveTextContent("posts");
  });

  it("switching tables resets view to data", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    // Go to SQL view first.
    await user.click(screen.getByText("SQL"));
    expect(screen.getByTestId("sql-editor")).toBeInTheDocument();

    // Click another table — should go back to data.
    await user.click(screen.getByText("users"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("switching tables from Search Settings resets view to data", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    await user.click(screen.getByText("Search Settings"));
    expect(screen.getByTestId("search-settings-view")).toHaveTextContent("posts");

    await user.click(screen.getByText("users"));
    expect(screen.getByTestId("table-browser")).toHaveTextContent("users");
  });

  it("calls onLogout when logout button clicked", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByTitle("Log out"));
    expect(onLogout).toHaveBeenCalledOnce();
  });

  it("calls onRefresh when refresh button clicked", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByTitle("Refresh schema"));
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("toggles theme with keyboard from the sidebar action button", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    const toggle = screen.getByRole("button", { name: "Switch to dark mode" });
    toggle.focus();
    await user.keyboard("{Enter}");

    expect(document.documentElement).toHaveClass("dark");
    expect(
      screen.getByRole("button", { name: "Switch to light mode" }),
    ).toBeInTheDocument();
  });

  it("shows schema prefix for non-public tables", () => {
    const schema = makeSchema({
      "other.events": { schema: "other", name: "events", kind: "table" },
    });
    renderWithTheme(
      <Layout schema={schema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    // "other." appears in sidebar and header, so use getAllByText.
    const prefixes = screen.getAllByText("other.");
    expect(prefixes.length).toBeGreaterThanOrEqual(1);
    for (const prefix of prefixes) {
      const classes = prefix.className.split(" ");
      expect(classes).toContain("text-gray-600");
      expect(classes).not.toContain("text-gray-400");
    }
    expect(screen.getAllByText("events").length).toBeGreaterThanOrEqual(1);
  });

  it("shows table kind badge in header", () => {
    const schema = makeSchema({
      "public.my_view": { schema: "public", name: "my_view", kind: "view" },
    });
    renderWithTheme(
      <Layout schema={schema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const badge = screen.getByText("view");
    expect(badge).toBeInTheDocument();
    const badgeClasses = badge.className.split(" ");
    expect(badgeClasses).toContain("text-gray-600");
    expect(badgeClasses).not.toContain("text-gray-500");
  });

  it("renders sidebar sections with Database, Services, and Admin groups", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Tables")).toBeInTheDocument();
    expect(screen.getByText("Database")).toBeInTheDocument();
    expect(screen.getByText("Services")).toBeInTheDocument();
    expect(screen.getByText("Admin")).toBeInTheDocument();
    expect(screen.getByText("Webhooks")).toBeInTheDocument();
    expect(screen.getByText("Storage")).toBeInTheDocument();
    expect(screen.getByText("Functions")).toBeInTheDocument();
    expect(screen.getByText("SQL Editor")).toBeInTheDocument();
    expect(screen.getByText("RLS Policies")).toBeInTheDocument();
  });

  it("sidebar section titles use WCAG AA compliant contrast tokens", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    // Section titles like "Database" should use text-gray-500 (4.56:1 on white)
    // not text-gray-400 (2.53:1) — check standalone token, not dark: prefix
    const databaseTitle = screen.getByText("Database");
    const classes = databaseTitle.className.split(" ");
    expect(classes).toContain("text-gray-500");
    expect(classes).not.toContain("text-gray-400");
  });

  it("switches to webhooks view on Webhooks click", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Webhooks"));
    expect(screen.getByTestId("webhooks-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("switches to storage view on Storage click", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Storage"));
    expect(screen.getByTestId("storage-view")).toBeInTheDocument();
  });

  it("clicking a table from admin view switches back to data view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();

    // Go to admin view first.
    await user.click(screen.getByText("Webhooks"));
    expect(screen.getByTestId("webhooks-view")).toBeInTheDocument();

    // Click a table — should return to data view.
    await user.click(screen.getByText("posts"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("switches to functions view on Functions click", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Functions"));
    expect(screen.getByTestId("functions-view")).toBeInTheDocument();
  });

  it("deselects table when switching to admin view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Storage"));
    // Header should not show table name.
    expect(screen.queryByTestId("table-browser")).not.toBeInTheDocument();
  });

  it("renders Messaging section in sidebar", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Messaging")).toBeInTheDocument();
    expect(screen.getByText("SMS Health")).toBeInTheDocument();
    expect(screen.getByText("SMS Messages")).toBeInTheDocument();
    expect(screen.getByText("Email Templates")).toBeInTheDocument();
    expect(screen.getByText("Push Notifications")).toBeInTheDocument();
  });

  it("clicking SMS Health renders SMSHealth component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SMS Health"));
    expect(screen.getByTestId("sms-health-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking SMS Messages renders SMSMessages component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("SMS Messages"));
    expect(screen.getByTestId("sms-messages-view")).toBeInTheDocument();
    // Tab bar should not be visible in admin views.
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking a table from SMS view returns to data view", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    // Go to SMS Health first.
    await user.click(screen.getByText("SMS Health"));
    expect(screen.getByTestId("sms-health-view")).toBeInTheDocument();
    // Click a table — should return to data view.
    await user.click(screen.getByText("posts"));
    expect(screen.getByTestId("table-browser")).toBeInTheDocument();
  });

  it("clicking Jobs renders Jobs component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Jobs"));
    expect(screen.getByTestId("jobs-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Schedules renders Schedules component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Schedules"));
    expect(screen.getByTestId("schedules-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Email Templates renders EmailTemplates component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Email Templates"));
    expect(screen.getByTestId("email-templates-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Push Notifications renders PushNotifications component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Push Notifications"));
    expect(screen.getByTestId("push-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("renders Auth section in sidebar with MFA and Account Linking items", () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    expect(screen.getByText("Auth")).toBeInTheDocument();
    expect(screen.getByText("Auth Settings")).toBeInTheDocument();
    expect(screen.getByText("MFA Management")).toBeInTheDocument();
    expect(screen.getByText("Account Linking")).toBeInTheDocument();
  });

  it("clicking MFA Management renders MFAEnrollment component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("MFA Management"));
    expect(screen.getByTestId("mfa-enrollment-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Account Linking renders AccountLinking component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Account Linking"));
    expect(screen.getByTestId("account-linking-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Realtime Inspector renders RealtimeInspector component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Realtime Inspector"));
    expect(screen.getByTestId("realtime-inspector-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Admin Logs renders AdminLogs component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Admin Logs"));
    expect(screen.getByTestId("admin-logs-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Usage renders UsageMetering component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Usage"));
    expect(screen.getByTestId("usage-metering-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Tenants renders Tenants component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Tenants"));
    expect(screen.getByTestId("tenants-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("clicking Organizations renders Organizations component", async () => {
    renderWithTheme(
      <Layout schema={twoTableSchema} onLogout={onLogout} onRefresh={onRefresh} />,
    );
    const user = userEvent.setup();
    await user.click(screen.getByText("Organizations"));
    expect(screen.getByTestId("organizations-view")).toBeInTheDocument();
    expect(screen.queryByText("Data")).not.toBeInTheDocument();
  });

  it("applies one capability-filtered registry to sidebar and command palette", async () => {
    vi.resetModules();
    const Icon = () => null;
    const mockedCapability = { kind: "known" as const, capabilities: { storage: false } };
    vi.doMock("../../capabilities", () => ({
      useCapability: () => ({
        state: mockedCapability,
        canUse: (capability: string) => capability !== "storage",
      }),
    }));
    vi.doMock("../../screens/registry", async () => {
      const actual =
        await vi.importActual<typeof import("../../screens/registry")>("../../screens/registry");
      return {
        ...actual,
        SCREEN_REGISTRY: {
          sections: [
            {
              title: "Services",
              screens: [
                {
                  id: "storage",
                  label: "Gated Storage",
                  icon: Icon,
                  requires: "storage",
                  render: () => <div data-testid="gated-storage" />,
                },
              ],
            },
            {
              title: "Admin",
              screens: [
                {
                  id: "users",
                  label: "Users",
                  icon: Icon,
                  render: () => <div data-testid="fixture-users" />,
                },
              ],
            },
          ],
        },
      };
    });
    const [{ Layout: MockedLayout }, { ThemeProvider: MockedThemeProvider }] =
      await Promise.all([
        import("../Layout"),
        import("../ThemeProvider"),
      ]);

    render(
      <MockedThemeProvider>
        <MockedLayout schema={makeSchema()} onLogout={onLogout} onRefresh={onRefresh} />
      </MockedThemeProvider>,
    );

    expect(screen.queryByRole("button", { name: "Gated Storage" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Users" })).toBeInTheDocument();

    await userEvent.setup().click(screen.getByRole("button", { name: /Search/ }));

    expect(screen.queryByText("Gated Storage")).not.toBeInTheDocument();
    expect(screen.getAllByText("Users").length).toBeGreaterThanOrEqual(2);

    vi.doUnmock("../../capabilities");
    vi.doUnmock("../../screens/registry");
  });
});
