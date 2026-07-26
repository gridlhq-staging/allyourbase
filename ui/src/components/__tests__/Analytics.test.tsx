import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, screen, fireEvent, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithProviders } from "../../test-utils";
import { Analytics } from "../Analytics";

vi.mock("../../api_analytics", () => ({
  listRequestLogs: vi.fn(),
  listRequestLogAggregates: vi.fn(),
  listAllRequestLogs: vi.fn(),
  streamRequestLogs: vi.fn(),
  listQueryStats: vi.fn(),
}));

import * as api from "../../api_analytics";

const mockRequestLogs = {
  items: [
    {
      id: "request-log-page-unique-001",
      timestamp: "2026-03-12T14:00:00Z",
      method: "PATCH",
      path: "/api/request-log-page-unique",
      status_code: 207,
      duration_ms: 1234.5,
      user_id: "user-page-unique-001",
      api_key_id: "api-key-page-unique-001",
      ip_address: "198.51.100.42",
      request_id: "request-id-page-unique-001",
      request_size: 1536,
      response_size: 2048,
    },
    {
      id: "r-2",
      timestamp: "2026-03-12T14:01:00Z",
      method: "POST",
      path: "/api/records",
      status_code: 500,
      duration_ms: 150,
      request_size: 0,
      response_size: 0,
    },
  ],
  count: 2,
  limit: 100,
  offset: 0,
};

const mockQueryStats = {
  items: [
    {
      queryid: "q-1",
      query: "SELECT * FROM users",
      calls: 1000,
      total_exec_time: 5000.5,
      mean_exec_time: 5.0,
      rows: 50000,
      shared_blks_hit: 8000,
      shared_blks_read: 200,
      index_suggestions: [
        { statement: "CREATE INDEX idx_users_email ON users(email)", confidence: "high" },
      ],
    },
  ],
  count: 1,
  limit: 50,
  sort: "total_time",
};

function requestLogRow(id: string, path = `/api/requests/${id}`) {
  return {
    ...mockRequestLogs.items[0],
    id,
    path,
    request_id: `request-id-${id}`,
  };
}

function requestLogPage(
  items: ReturnType<typeof requestLogRow>[],
  count: number,
  offset: number,
) {
  return {
    items,
    count,
    limit: 25,
    offset,
  };
}

interface StreamHandlers {
  signal?: AbortSignal;
  onReady?: () => void;
  onRequestLog: (row: ReturnType<typeof requestLogRow>) => void;
}

interface StreamCall {
  filters: Record<string, string | undefined>;
  handlers: StreamHandlers;
  aborted: boolean;
}

let streamCalls: StreamCall[] = [];

async function readBlob(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.addEventListener("load", () => resolve(String(reader.result)));
    reader.addEventListener("error", () => reject(reader.error));
    reader.readAsText(blob);
  });
}

function captureDownloads() {
  const originalCreateObjectURL = URL.createObjectURL;
  const originalRevokeObjectURL = URL.revokeObjectURL;
  const createObjectURL = vi.fn(() => "blob:analytics-export");
  const revokeObjectURL = vi.fn();
  const downloads: HTMLAnchorElement[] = [];
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    value: createObjectURL,
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    value: revokeObjectURL,
  });
  const click = vi
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(function captureDownload() {
      downloads.push(this);
    });

  return {
    createObjectURL,
    revokeObjectURL,
    downloads,
    restore() {
      click.mockRestore();
      Object.defineProperty(URL, "createObjectURL", {
        configurable: true,
        value: originalCreateObjectURL,
      });
      Object.defineProperty(URL, "revokeObjectURL", {
        configurable: true,
        value: originalRevokeObjectURL,
      });
    },
  };
}

beforeEach(() => {
  vi.resetAllMocks();
  streamCalls = [];
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText: vi.fn().mockResolvedValue(undefined) },
  });
  (api.listRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(mockRequestLogs);
  (api.listRequestLogAggregates as ReturnType<typeof vi.fn>).mockResolvedValue({
    items: [],
    count: 0,
  });
  (api.listAllRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(mockRequestLogs);
  (api.streamRequestLogs as ReturnType<typeof vi.fn>).mockImplementation(
    (filters: Record<string, string | undefined>, handlers: StreamHandlers) => {
      const call: StreamCall = { filters, handlers, aborted: false };
      streamCalls.push(call);
      handlers.signal?.addEventListener("abort", () => {
        call.aborted = true;
      });
      return new Promise<void>((resolve) => {
        handlers.signal?.addEventListener("abort", () => resolve(), { once: true });
      });
    },
  );
  (api.listQueryStats as ReturnType<typeof vi.fn>).mockResolvedValue(mockQueryStats);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Analytics", () => {
  it("renders every request-log table column with exact row values", async () => {
    renderWithProviders(<Analytics />);

    const table = await screen.findByTestId("request-logs-table");
    for (const header of ["Time", "Method", "Path", "Status", "Duration", "Size", "Identity"]) {
      expect(within(table).getByRole("columnheader", { name: header })).toBeInTheDocument();
    }

    const primaryRow = within(table).getByTestId("request-log-row-request-log-page-unique-001");
    expect(within(primaryRow).getByText(new Date("2026-03-12T14:00:00Z").toLocaleString())).toBeInTheDocument();
    expect(within(primaryRow).getByText("PATCH")).toBeInTheDocument();
    expect(within(primaryRow).getByText("/api/request-log-page-unique")).toBeInTheDocument();
    expect(within(primaryRow).getByText("207")).toBeInTheDocument();
    expect(within(primaryRow).getByText("1234.5ms")).toBeInTheDocument();
    expect(within(primaryRow).getByText("1.5 KB / 2.0 KB")).toBeInTheDocument();
    expect(within(primaryRow).getByText("User user-page-unique-001")).toBeInTheDocument();

    const missingIdentityRow = within(table).getByTestId("request-log-row-r-2");
    expect(within(missingIdentityRow).getByText("-", { exact: true })).toBeInTheDocument();
    expect(within(missingIdentityRow).getByText("0 B / 0 B")).toBeInTheDocument();
  });

  it("opens, copies from, and closes the complete request details drawer", async () => {
    renderWithProviders(<Analytics />);

    const rowAction = await screen.findByTestId(
      "request-log-view-details-request-log-page-unique-001",
    );
    rowAction.focus();
    fireEvent.click(rowAction);

    const dialog = screen.getByRole("dialog", { name: "Request details" });
    expect(dialog).toHaveAttribute("data-testid", "request-log-drawer");
    const expectedDetails: Record<string, string> = {
      "request-log-detail-id": "request-log-page-unique-001",
      "request-log-detail-timestamp": "2026-03-12T14:00:00Z",
      "request-log-detail-method": "PATCH",
      "request-log-detail-path": "/api/request-log-page-unique",
      "request-log-detail-status-code": "207",
      "request-log-detail-duration-ms": "1234.5ms",
      "request-log-detail-user-id": "user-page-unique-001",
      "request-log-detail-api-key-id": "api-key-page-unique-001",
      "request-log-detail-ip-address": "198.51.100.42",
      "request-log-detail-request-id": "request-id-page-unique-001",
      "request-log-detail-request-size": "1.5 KB (1536 bytes)",
      "request-log-detail-response-size": "2.0 KB (2048 bytes)",
    };
    for (const [testId, value] of Object.entries(expectedDetails)) {
      expect(within(dialog).getByTestId(testId)).toHaveTextContent(value);
    }

    fireEvent.click(within(dialog).getByTestId("request-log-copy-request-id"));
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      "request-id-page-unique-001",
    );

    fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    await waitFor(() => expect(rowAction).toHaveFocus());
    expect(screen.queryByText("198.51.100.42")).not.toBeInTheDocument();
    expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request Logs" })).toBeInTheDocument();

    fireEvent.click(rowAction);
    const escapeDialog = screen.getByRole("dialog", { name: "Request details" });
    fireEvent.keyDown(escapeDialog, {
      key: "Escape",
    });
    expect(screen.queryByRole("dialog", { name: "Request details" })).not.toBeInTheDocument();
    expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Request Logs" })).toBeInTheDocument();

    fireEvent.click(rowAction);
    const focusTrapDialog = screen.getByRole("dialog", { name: "Request details" });
    const closeButton = within(focusTrapDialog).getByRole("button", { name: "Close" });
    const copyButton = within(focusTrapDialog).getByTestId("request-log-copy-request-id");
    expect(closeButton).toHaveFocus();
    fireEvent.keyDown(closeButton, { key: "Tab", shiftKey: true });
    expect(copyButton).toHaveFocus();
    fireEvent.keyDown(copyButton, { key: "Tab" });
    expect(closeButton).toHaveFocus();

    fireEvent.mouseDown(focusTrapDialog.parentElement!);
    expect(screen.queryByRole("dialog", { name: "Request details" })).not.toBeInTheDocument();
    await waitFor(() => expect(rowAction).toHaveFocus());
  });

  it("opens request details from row click, Enter, and Space", async () => {
    renderWithProviders(<Analytics />);
    const row = await screen.findByTestId("request-log-row-r-2");

    for (const open of [
      () => fireEvent.click(row),
      () => fireEvent.keyDown(row, { key: "Enter" }),
      () => fireEvent.keyDown(row, { key: " " }),
    ]) {
      open();
      const dialog = screen.getByRole("dialog", { name: "Request details" });
      expect(within(dialog).getByTestId("request-log-detail-user-id")).toHaveTextContent("-");
      expect(within(dialog).getByTestId("request-log-detail-api-key-id")).toHaveTextContent("-");
      expect(within(dialog).getByTestId("request-log-detail-ip-address")).toHaveTextContent("-");
      expect(within(dialog).getByTestId("request-log-detail-request-id")).toHaveTextContent("-");
      expect(within(dialog).getByTestId("request-log-copy-request-id")).toBeDisabled();
      fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
    }
  });

  it("keeps keyboard focus origin on the View details action", async () => {
    const user = userEvent.setup();
    renderWithProviders(<Analytics />);

    const rowAction = await screen.findByTestId(
      "request-log-view-details-request-log-page-unique-001",
    );

    for (const key of ["{Enter}", " "]) {
      rowAction.focus();
      await user.keyboard(key);

      const dialog = screen.getByRole("dialog", { name: "Request details" });
      fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));

      await waitFor(() => expect(rowAction).toHaveFocus());
    }
  });

  it("switches to query performance tab", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(screen.getByText("SELECT * FROM users")).toBeInTheDocument();
    });
  });

  it("applies request-log filters including status code and date range", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    });

    fireEvent.change(screen.getByLabelText("Method"), {
      target: { value: "GET" },
    });
    fireEvent.change(screen.getByLabelText("Path"), {
      target: { value: "/api/users*" },
    });
    fireEvent.change(screen.getByLabelText("Status Code"), {
      target: { value: "200" },
    });
    fireEvent.change(screen.getByLabelText("From"), {
      target: { value: "2026-03-01" },
    });
    fireEvent.change(screen.getByLabelText("To"), {
      target: { value: "2026-03-12" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenLastCalledWith({
        method: "GET",
        path: "/api/users*",
        status: "200",
        statusClass: undefined,
        minDurationMs: undefined,
        maxDurationMs: undefined,
        from: "2026-03-01",
        to: "2026-03-12",
        limit: 25,
        offset: 0,
      });
    });
  });

  it("keeps status-class and latency edits draft-only until Apply", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Status Class"), {
      target: { value: "4xx" },
    });
    fireEvent.change(screen.getByLabelText("Minimum Latency (ms)"), {
      target: { value: "25" },
    });
    fireEvent.change(screen.getByLabelText("Maximum Latency (ms)"), {
      target: { value: "750" },
    });
    expect(api.listRequestLogs).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(2);
    });
    expect(api.listRequestLogs).toHaveBeenLastCalledWith({
      method: undefined,
      path: undefined,
      status: undefined,
      statusClass: "4xx",
      minDurationMs: "25",
      maxDurationMs: "750",
      from: undefined,
      to: undefined,
      limit: 25,
      offset: 0,
    });
  });

  it.each([
    {
      minimum: "-1",
      maximum: "",
      message: "Minimum latency must be zero or greater",
    },
    {
      minimum: "20",
      maximum: "10",
      message: "Minimum latency must be less than or equal to maximum latency",
    },
    {
      minimum: "1.5",
      maximum: "",
      message: "Minimum latency must be a non-negative whole number",
    },
    {
      minimum: "",
      maximum: "1e3",
      message: "Maximum latency must be a non-negative whole number",
    },
    {
      minimum: "9223372036854775808",
      maximum: "",
      message: "Minimum latency must be no greater than 9223372036854775807",
    },
  ])("rejects invalid latency filters: $message", async ({ minimum, maximum, message }) => {
    renderWithProviders(<Analytics />);
    await waitFor(() => expect(api.listRequestLogs).toHaveBeenCalledTimes(1));

    fireEvent.change(screen.getByLabelText("Minimum Latency (ms)"), {
      target: { value: minimum },
    });
    fireEvent.change(screen.getByLabelText("Maximum Latency (ms)"), {
      target: { value: maximum },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(message);
    expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
  });

  it("shows total-driven ranges and pages to page-unique content", async () => {
    const firstPageRows = Array.from({ length: 25 }, (_, index) =>
      requestLogRow(`page-one-${index + 1}`),
    );
    const secondPageRow = requestLogRow("page-two-unique");
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(requestLogPage(firstPageRows, 26, 0))
      .mockResolvedValueOnce(requestLogPage([secondPageRow], 26, 25));

    renderWithProviders(<Analytics />);

    expect(await screen.findByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 1–25 of 26 request logs",
    );
    const pager = screen.getByTestId("request-logs-pager");
    expect(
      within(pager).getByRole("button", { name: "Previous request-log page" }),
    ).toBeDisabled();
    const nextButton = within(pager).getByRole("button", {
      name: "Next request-log page",
    });
    expect(nextButton).toBeEnabled();

    fireEvent.click(nextButton);

    expect(await screen.findByTestId("request-log-row-page-two-unique")).toBeInTheDocument();
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 25, offset: 25 }),
    );
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 26–26 of 26 request logs",
    );
    expect(
      within(pager).getByRole("button", { name: "Previous request-log page" }),
    ).toBeEnabled();
    expect(
      within(pager).getByRole("button", { name: "Next request-log page" }),
    ).toBeDisabled();
  });

  it("applying filters from a later page resets to offset zero", async () => {
    const firstPage = requestLogPage([requestLogRow("first-page")], 40, 0);
    const laterPage = requestLogPage([requestLogRow("later-page")], 40, 25);
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(firstPage)
      .mockResolvedValueOnce(laterPage)
      .mockResolvedValueOnce(requestLogPage([requestLogRow("filtered")], 1, 0));

    renderWithProviders(<Analytics />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Next request-log page" }),
    );
    await screen.findByTestId("request-log-row-later-page");

    fireEvent.change(screen.getByLabelText("Path"), {
      target: { value: "/api/filtered*" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Apply Filters" }));

    await screen.findByTestId("request-log-row-filtered");
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ path: "/api/filtered*", offset: 0 }),
    );
  });

  it("Reset clears every filter and returns to page one", async () => {
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(requestLogPage([requestLogRow("first-page")], 40, 0))
      .mockResolvedValueOnce(requestLogPage([requestLogRow("later-page")], 40, 25))
      .mockResolvedValueOnce(requestLogPage([requestLogRow("reset-page")], 1, 0));

    renderWithProviders(<Analytics />);
    await screen.findByTestId("request-log-row-first-page");
    for (const [label, value] of [
      ["Method", "POST"],
      ["Path", "/api/reset*"],
      ["Status Code", "201"],
      ["Status Class", "2xx"],
      ["From", "2026-03-01"],
      ["To", "2026-03-12"],
      ["Minimum Latency (ms)", "10"],
      ["Maximum Latency (ms)", "20"],
    ]) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
    }
    fireEvent.click(screen.getByRole("button", { name: "Next request-log page" }));
    await screen.findByTestId("request-log-row-later-page");

    fireEvent.click(screen.getByRole("button", { name: "Reset" }));

    await screen.findByTestId("request-log-row-reset-page");
    for (const [label, displayValue] of [
      ["Method", "All methods"],
      ["Path", ""],
      ["Status Code", ""],
      ["Status Class", "All status classes"],
      ["From", ""],
      ["To", ""],
      ["Minimum Latency (ms)", ""],
      ["Maximum Latency (ms)", ""],
    ]) {
      expect(screen.getByLabelText(label)).toHaveDisplayValue(displayValue);
    }
    expect(api.listRequestLogs).toHaveBeenLastCalledWith({
      method: undefined,
      path: undefined,
      status: undefined,
      statusClass: undefined,
      minDurationMs: undefined,
      maxDurationMs: undefined,
      from: undefined,
      to: undefined,
      limit: 25,
      offset: 0,
    });
  });

  it("corrects a stale nonempty page and preserves the corrected page across tabs", async () => {
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(requestLogPage([requestLogRow("page-one")], 40, 0))
      .mockResolvedValueOnce(requestLogPage([requestLogRow("stale-page")], 10, 25))
      .mockResolvedValue(requestLogPage([requestLogRow("corrected-page")], 10, 0));

    renderWithProviders(<Analytics />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Next request-log page" }),
    );

    await screen.findByTestId("request-log-row-corrected-page");
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 0 }),
    );
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 1–1 of 10 request logs",
    );

    fireEvent.click(screen.getByRole("button", { name: "Query Performance" }));
    await screen.findByText("SELECT * FROM users");
    fireEvent.click(screen.getByRole("button", { name: "Request Logs" }));

    expect(await screen.findByTestId("request-log-row-corrected-page")).toBeInTheDocument();
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 0 }),
    );
  });

  it("resets a stale empty page to offset zero", async () => {
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(requestLogPage([requestLogRow("page-one")], 40, 0))
      .mockResolvedValueOnce(requestLogPage([], 0, 25))
      .mockResolvedValue(requestLogPage([], 0, 0));

    renderWithProviders(<Analytics />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Next request-log page" }),
    );

    expect(await screen.findByText("No request logs found")).toBeInTheDocument();
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ offset: 0 }),
    );
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 0–0 of 0 request logs",
    );
    expect(
      within(screen.getByTestId("request-logs-pager")).getByRole("button", {
        name: "Previous request-log page",
      }),
    ).toBeDisabled();
  });

  it("streams live request logs into the newest-first table without a page reload", async () => {
    renderWithProviders(<Analytics />);
    await screen.findByTestId("request-logs-table");

    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));
    await waitFor(() => expect(api.streamRequestLogs).toHaveBeenCalledTimes(1));
    expect(screen.getByTestId("request-logs-live-status")).toHaveTextContent("Connecting");

    await act(async () => {
      streamCalls[0].handlers.onReady?.();
    });
    await waitFor(() => {
      expect(screen.getByTestId("request-logs-live-status")).toHaveTextContent("Live");
    });

    await act(async () => {
      streamCalls[0].handlers.onRequestLog({
        ...requestLogRow("live-row", "/api/live-row"),
        method: "GET",
        status_code: 404,
        duration_ms: 77,
      });
    });

    const tableRows = within(screen.getByTestId("request-logs-table")).getAllByRole("row");
    expect(tableRows[1]).toHaveTextContent("/api/live-row");
    expect(tableRows[1]).toHaveTextContent("GET");
    expect(tableRows[1]).toHaveTextContent("404");
    expect(tableRows[1]).toHaveTextContent("77ms");
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 1–3 of 3 request logs",
    );
    expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
  });

  it("deduplicates streamed IDs, caps the current page, and increments count once per accepted row", async () => {
    const pageRows = Array.from({ length: 25 }, (_, index) =>
      requestLogRow(`live-cap-${index}`),
    );
    (api.listRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(
      requestLogPage(pageRows, 25, 0),
    );

    renderWithProviders(<Analytics />);
    await screen.findByTestId("request-log-row-live-cap-0");
    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));
    await waitFor(() => expect(api.streamRequestLogs).toHaveBeenCalledTimes(1));

    await act(async () => {
      streamCalls[0].handlers.onRequestLog(pageRows[0]);
      streamCalls[0].handlers.onRequestLog(requestLogRow("accepted-live-cap"));
    });

    const bodyRows = within(screen.getByTestId("request-logs-table")).getAllByRole("row").slice(1);
    expect(bodyRows).toHaveLength(25);
    expect(screen.getByTestId("request-log-row-accepted-live-cap")).toBeInTheDocument();
    expect(screen.queryAllByTestId("request-log-row-live-cap-0")).toHaveLength(1);
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 1–25 of 26 request logs",
    );
  });

  it("keeps streamed rows when the offset-zero reload finishes after live delivery", async () => {
    const firstPageRows = Array.from({ length: 25 }, (_, index) =>
      requestLogRow(`reload-page-one-${index}`),
    );
    const secondPageRows = [requestLogRow("reload-page-two-unique")];
    let resolveResetToNewestPage:
      | ((value: ReturnType<typeof requestLogPage>) => void)
      | undefined;
    const resetToNewestPage = new Promise<ReturnType<typeof requestLogPage>>((resolve) => {
      resolveResetToNewestPage = resolve;
    });
    (api.listRequestLogs as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce(requestLogPage(firstPageRows, 26, 0))
      .mockResolvedValueOnce(requestLogPage(secondPageRows, 26, 25))
      .mockImplementationOnce(() => resetToNewestPage);

    renderWithProviders(<Analytics />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Next request-log page" }),
    );
    await screen.findByTestId("request-log-row-reload-page-two-unique");

    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));
    await waitFor(() => expect(streamCalls).toHaveLength(1));

    await act(async () => {
      streamCalls[0].handlers.onRequestLog(
        requestLogRow("reload-live-row", "/api/reload-live-row"),
      );
    });
    expect(screen.getByTestId("request-log-row-reload-live-row")).toBeInTheDocument();

    await act(async () => {
      resolveResetToNewestPage?.(requestLogPage(firstPageRows, 26, 0));
      await resetToNewestPage;
    });

    await waitFor(() => {
      expect(screen.getByTestId("request-log-row-reload-live-row")).toBeInTheDocument();
    });
    const tableRows = within(screen.getByTestId("request-logs-table")).getAllByRole("row");
    expect(tableRows[1]).toHaveTextContent("/api/reload-live-row");
    expect(screen.queryByTestId("request-log-row-reload-page-two-unique")).not.toBeInTheDocument();
    expect(screen.getByTestId("request-logs-summary")).toHaveTextContent(
      "Showing 1–25 of 27 request logs",
    );
  });

  it("reconnects live streaming only when applied filters change", async () => {
    renderWithProviders(<Analytics />);
    await screen.findByTestId("request-logs-table");
    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));
    await waitFor(() => expect(streamCalls).toHaveLength(1));

    fireEvent.change(screen.getByLabelText("Path"), {
      target: { value: "/api/draft-only*" },
    });
    expect(streamCalls).toHaveLength(1);
    expect(streamCalls[0].aborted).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: "Apply Filters" }));
    await waitFor(() => expect(streamCalls).toHaveLength(2));
    expect(streamCalls[0].aborted).toBe(true);
    expect(streamCalls[1].filters).toMatchObject({ path: "/api/draft-only*" });
    expect(api.listRequestLogs).toHaveBeenLastCalledWith(
      expect.objectContaining({ path: "/api/draft-only*", offset: 0 }),
    );
  });

  it("preserves visible rows and reports live stream errors", async () => {
    (api.streamRequestLogs as ReturnType<typeof vi.fn>).mockImplementationOnce(
      (filters: Record<string, string | undefined>, handlers: StreamHandlers) => {
        const call: StreamCall = { filters, handlers, aborted: false };
        streamCalls.push(call);
        handlers.signal?.addEventListener("abort", () => {
          call.aborted = true;
        });
        return Promise.reject(new Error("live stream failed"));
      },
    );

    renderWithProviders(<Analytics />);
    await screen.findByText("/api/request-log-page-unique");
    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));

    expect(await screen.findByTestId("request-logs-live-status")).toHaveTextContent(
      "Error: live stream failed",
    );
    expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
  });

  it("aborts live streams on toggle off, tab change, and unmount while preserving rows on errors", async () => {
    const { unmount } = renderWithProviders(<Analytics />);
    await screen.findByTestId("request-logs-table");
    const liveButton = screen.getByRole("button", { name: "Live (periodic refresh)" });

    fireEvent.click(liveButton);
    await waitFor(() => expect(streamCalls).toHaveLength(1));
    fireEvent.click(liveButton);
    expect(streamCalls[0].aborted).toBe(true);
    expect(screen.getByTestId("request-logs-live-status")).toHaveTextContent("Off");
    expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();

    fireEvent.click(liveButton);
    await waitFor(() => expect(streamCalls).toHaveLength(2));
    fireEvent.click(screen.getByRole("button", { name: "Query Performance" }));
    expect(streamCalls[1].aborted).toBe(true);
    await screen.findByText("SELECT * FROM users");

    fireEvent.click(screen.getByRole("button", { name: "Request Logs" }));
    expect(await screen.findByTestId("request-logs-live-status")).toHaveTextContent("Off");
    fireEvent.click(screen.getByRole("button", { name: "Live (periodic refresh)" }));
    await waitFor(() => expect(streamCalls).toHaveLength(3));
    unmount();
    expect(streamCalls[2].aborted).toBe(true);
  });

  it.each([
    {
      format: "JSON",
      mime: "application/json",
      extension: "json",
      expectedContent: JSON.stringify([
        {
          id: "export-special",
          timestamp: "2026-03-12T14:00:00Z",
          method: "POST",
          path: "=HYPERLINK(\"https://example.invalid\",\"quoted\")\nnext",
          status_code: 201,
          duration_ms: 42,
          user_id: "export-user",
          api_key_id: "export-key",
          request_size: 12,
          response_size: 34,
          ip_address: "192.0.2.10",
          request_id: "export-request",
        },
      ], null, 2),
    },
    {
      format: "CSV",
      mime: "text/csv",
      extension: "csv",
      expectedContent:
        "id,timestamp,method,path,status_code,duration_ms,user_id,api_key_id,request_size,response_size,ip_address,request_id\n" +
        "export-special,2026-03-12T14:00:00Z,POST,\"'=HYPERLINK(\"\"https://example.invalid\"\",\"\"quoted\"\")\nnext\",201,42,export-user,export-key,12,34,192.0.2.10,export-request",
    },
  ])("exports exact $format Blob content and filename", async ({
    format,
    mime,
    extension,
    expectedContent,
  }) => {
    const exportRow = {
      id: "export-special",
      timestamp: "2026-03-12T14:00:00Z",
      method: "POST",
      path: "=HYPERLINK(\"https://example.invalid\",\"quoted\")\nnext",
      status_code: 201,
      duration_ms: 42,
      user_id: "export-user",
      api_key_id: "export-key",
      request_size: 12,
      response_size: 34,
      ip_address: "192.0.2.10",
      request_id: "export-request",
    };
    (api.listAllRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(
      requestLogPage([exportRow], 1, 0),
    );
    const downloads = captureDownloads();
    vi.spyOn(Date.prototype, "toISOString").mockReturnValue(
      "2026-03-12T14:00:00.000Z",
    );

    try {
      renderWithProviders(<Analytics />);
      await screen.findByTestId("request-logs-table");
      fireEvent.click(screen.getByRole("button", { name: `Export ${format}` }));

      await waitFor(() => expect(downloads.createObjectURL).toHaveBeenCalledTimes(1));
      const blob = downloads.createObjectURL.mock.calls[0][0] as Blob;
      expect(blob.type).toBe(mime);
      expect(await readBlob(blob)).toBe(expectedContent);
      expect(downloads.downloads[0]?.download).toBe(
        `request_logs_2026-03-12T14-00-00-000Z.${extension}`,
      );
      expect(downloads.revokeObjectURL).toHaveBeenCalledWith(
        "blob:analytics-export",
      );
    } finally {
      downloads.restore();
    }
  });

  it("exports all matches using only applied filters and independent pending state", async () => {
    let resolveExport: ((value: typeof mockRequestLogs) => void) | undefined;
    (api.listAllRequestLogs as ReturnType<typeof vi.fn>).mockImplementation(
      () => new Promise((resolve) => {
        resolveExport = resolve;
      }),
    );
    const downloads = captureDownloads();

    try {
      renderWithProviders(<Analytics />);
      await screen.findByTestId("request-logs-table");
      fireEvent.change(screen.getByLabelText("Path"), {
        target: { value: "/api/draft-only*" },
      });

      fireEvent.click(screen.getByRole("button", { name: "Export JSON" }));

      expect(api.listAllRequestLogs).toHaveBeenCalledWith({
        method: undefined,
        path: undefined,
        status: undefined,
        statusClass: undefined,
        minDurationMs: undefined,
        maxDurationMs: undefined,
        from: undefined,
        to: undefined,
      });
      expect(screen.getByRole("button", { name: "Exporting JSON..." })).toBeDisabled();
      expect(screen.getByRole("button", { name: "Export CSV" })).toBeEnabled();
      expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();

      resolveExport?.({
        ...mockRequestLogs,
        items: [...mockRequestLogs.items, requestLogRow("beyond-visible-page")],
        count: 3,
      });
      await waitFor(() => expect(downloads.createObjectURL).toHaveBeenCalledTimes(1));
      const exported = await readBlob(
        downloads.createObjectURL.mock.calls[0][0] as Blob,
      );
      expect(exported).toContain("beyond-visible-page");
      expect(screen.getByRole("button", { name: "Export JSON" })).toBeEnabled();
    } finally {
      downloads.restore();
    }
  });

  it("does not download an empty export and reports the deterministic message", async () => {
    (api.listAllRequestLogs as ReturnType<typeof vi.fn>).mockResolvedValue(
      requestLogPage([], 0, 0),
    );
    const downloads = captureDownloads();

    try {
      renderWithProviders(<Analytics />);
      await screen.findByTestId("request-logs-table");
      fireEvent.click(screen.getByRole("button", { name: "Export CSV" }));

      expect(
        await screen.findByText("No matching request logs to export"),
      ).toBeInTheDocument();
      expect(downloads.createObjectURL).not.toHaveBeenCalled();
    } finally {
      downloads.restore();
    }
  });

  it.each([
    {
      arrange: () =>
        (api.listAllRequestLogs as ReturnType<typeof vi.fn>).mockRejectedValue(
          new Error("export fetch failed"),
        ),
      expected: "export fetch failed",
    },
    {
      arrange: () => vi.stubGlobal("Blob", class {
        constructor() {
          throw new Error("blob creation failed");
        }
      }),
      expected: "blob creation failed",
    },
  ])("surfaces export failures through toasts: $expected", async ({
    arrange,
    expected,
  }) => {
    arrange();
    renderWithProviders(<Analytics />);
    await screen.findByTestId("request-logs-table");

    fireEvent.click(screen.getByRole("button", { name: "Export JSON" }));

    expect(await screen.findByText(expected)).toBeInTheDocument();
    vi.unstubAllGlobals();
  });

  it("updates query stats sort parameter when sort dropdown changes", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(api.listQueryStats).toHaveBeenLastCalledWith({ sort: "total_time" });
    });

    fireEvent.change(screen.getByLabelText("Sort by"), {
      target: { value: "calls" },
    });
    await waitFor(() => {
      expect(api.listQueryStats).toHaveBeenLastCalledWith({ sort: "calls" });
    });
  });

  it("displays index suggestions in query stats tab", async () => {
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("/api/request-log-page-unique")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /query performance/i }));
    await waitFor(() => {
      expect(
        screen.getByText("CREATE INDEX idx_users_email ON users(email)"),
      ).toBeInTheDocument();
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listRequestLogs as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("Failed to load"),
    );
    renderWithProviders(<Analytics />);
    await waitFor(() => {
      expect(screen.getByText("Failed to load")).toBeInTheDocument();
    });
  });
});
