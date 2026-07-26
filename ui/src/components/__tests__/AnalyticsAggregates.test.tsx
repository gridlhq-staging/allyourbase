import { fireEvent, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { renderWithProviders } from "../../test-utils";
import { Analytics } from "../Analytics";

vi.mock("../../api_analytics", () => ({
  listRequestLogs: vi.fn(),
  listRequestLogAggregates: vi.fn(),
  listAllRequestLogs: vi.fn(),
  listQueryStats: vi.fn(),
}));

import * as api from "../../api_analytics";

const AGGREGATE_RESPONSE = {
  items: [
    {
      bucket: "2026-07-26T12:00:00Z",
      count: 7,
      status_2xx: 2,
      status_3xx: 1,
      status_4xx: 3,
      status_5xx: 1,
    },
  ],
  count: 1,
};

const REQUEST_LOG_ENTRY = {
  id: "request-log-1",
  timestamp: "2026-07-26T12:00:00Z",
  method: "GET",
  path: "/api/orders",
  status_code: 200,
  duration_ms: 12,
  request_size: 100,
  response_size: 200,
};

beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.listRequestLogs).mockResolvedValue({
    items: [REQUEST_LOG_ENTRY],
    count: 50,
    limit: 25,
    offset: 0,
  });
  vi.mocked(api.listRequestLogAggregates).mockResolvedValue(
    AGGREGATE_RESPONSE,
  );
  vi.mocked(api.listAllRequestLogs).mockResolvedValue({
    items: [],
    count: 0,
    limit: 500,
    offset: 0,
  });
  vi.mocked(api.listQueryStats).mockResolvedValue({
    items: [],
    count: 0,
    limit: 50,
    sort: "total_time",
  });
});

describe("Analytics request-log aggregates", () => {
  it("loads chart data with the same applied filters as the request table", async () => {
    renderWithProviders(<Analytics />);

    await waitFor(() => {
      expect(api.listRequestLogAggregates).toHaveBeenCalledWith({
        method: undefined,
        path: undefined,
        status: undefined,
        statusClass: undefined,
        minDurationMs: undefined,
        maxDurationMs: undefined,
        from: undefined,
        to: undefined,
      });
    });
    expect(await screen.findByTestId("request-log-volume-bar-0")).toHaveAttribute(
      "data-count",
      "7",
    );

    for (const [label, value] of [
      ["Method", "POST"],
      ["Path", "/api/chart-orders/*"],
      ["Status Code", "418"],
      ["Status Class", "4xx"],
      ["Minimum Latency (ms)", "25"],
      ["Maximum Latency (ms)", "750"],
      ["From", "2026-07-26"],
      ["To", "2026-07-27"],
    ]) {
      fireEvent.change(screen.getByLabelText(label), { target: { value } });
    }
    fireEvent.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(api.listRequestLogAggregates).toHaveBeenLastCalledWith({
        method: "POST",
        path: "/api/chart-orders/*",
        status: "418",
        statusClass: "4xx",
        minDurationMs: "25",
        maxDurationMs: "750",
        from: "2026-07-26",
        to: "2026-07-27",
      });
    });
  });

  it("does not reload aggregates when request-table pagination changes", async () => {
    renderWithProviders(<Analytics />);

    await screen.findByTestId("request-log-volume-bar-0");
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
      expect(api.listRequestLogAggregates).toHaveBeenCalledTimes(1);
    });
    vi.mocked(api.listRequestLogAggregates).mockClear();

    fireEvent.click(
      screen.getByRole("button", { name: "Next request-log page" }),
    );

    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenLastCalledWith(
        expect.objectContaining({ offset: 25 }),
      );
    });
    expect(api.listRequestLogAggregates).not.toHaveBeenCalled();
  });

  it("retries chart loading without reloading the request table", async () => {
    vi.mocked(api.listRequestLogAggregates).mockRejectedValueOnce(
      new Error("Aggregate refresh failed"),
    );
    renderWithProviders(<Analytics />);

    await screen.findByText("Aggregate refresh failed");
    await waitFor(() => {
      expect(api.listRequestLogs).toHaveBeenCalledTimes(1);
      expect(api.listRequestLogAggregates).toHaveBeenCalledTimes(1);
    });
    vi.mocked(api.listRequestLogs).mockClear();
    vi.mocked(api.listRequestLogAggregates).mockResolvedValueOnce(
      AGGREGATE_RESPONSE,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Retry request charts" }),
    );

    await waitFor(() => {
      expect(api.listRequestLogAggregates).toHaveBeenCalledTimes(2);
    });
    expect(api.listRequestLogs).not.toHaveBeenCalled();
  });
});
