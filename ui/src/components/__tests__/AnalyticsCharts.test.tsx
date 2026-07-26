import { fireEvent, render, screen, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { RequestLogAggregateBucket } from "../../types/analytics";
import {
  AnalyticsCharts,
  MAX_CHART_BUCKETS,
  reduceAggregateBuckets,
} from "../AnalyticsCharts";

const MINUTE_MS = 60_000;

function minuteBuckets(
  startIso: string,
  minuteCount: number,
  countForMinute: (minuteIndex: number) => number,
): RequestLogAggregateBucket[] {
  const startMs = new Date(startIso).getTime();
  return Array.from({ length: minuteCount }, (_, minuteIndex) => ({
    bucket: new Date(startMs + minuteIndex * MINUTE_MS).toISOString(),
    count: countForMinute(minuteIndex),
    status_2xx: countForMinute(minuteIndex),
    status_3xx: 0,
    status_4xx: 0,
    status_5xx: 0,
  }));
}

const ITEMS: RequestLogAggregateBucket[] = [
  {
    bucket: "2026-07-26T12:00:00Z",
    count: 4,
    status_2xx: 2,
    status_3xx: 1,
    status_4xx: 1,
    status_5xx: 0,
  },
  {
    bucket: "2026-07-26T12:01:00Z",
    count: 8,
    status_2xx: 3,
    status_3xx: 0,
    status_4xx: 2,
    status_5xx: 3,
  },
];

describe("AnalyticsCharts", () => {
  it("encodes request counts and status classes in accessible SVG marks", () => {
    render(<AnalyticsCharts items={ITEMS} loading={false} error={null} />);

    const charts = screen.getByTestId("request-log-aggregate-charts");
    expect(
      within(charts).getByRole("img", { name: "Request volume by minute" }),
    ).toBeInTheDocument();
    expect(
      within(charts).getByRole("img", {
        name: "Request status classes by minute",
      }),
    ).toBeInTheDocument();

    expect(screen.getByTestId("request-log-volume-bar-0")).toHaveAttribute(
      "height",
      "60",
    );
    expect(screen.getByTestId("request-log-volume-bar-1")).toHaveAttribute(
      "height",
      "120",
    );
    expect(screen.getByTestId("request-log-volume-bar-1")).toHaveAttribute(
      "data-count",
      "8",
    );

    expect(screen.getByTestId("request-log-status-2xx-0")).toHaveAttribute(
      "data-count",
      "2",
    );
    expect(screen.getByTestId("request-log-status-3xx-0")).toHaveAttribute(
      "data-count",
      "1",
    );
    expect(screen.getByTestId("request-log-status-4xx-1")).toHaveAttribute(
      "data-count",
      "2",
    );
    expect(screen.getByTestId("request-log-status-5xx-1")).toHaveAttribute(
      "data-count",
      "3",
    );

    for (const statusClass of ["2xx", "3xx", "4xx", "5xx"]) {
      expect(
        within(screen.getByTestId("request-log-status-legend")).getByText(
          statusClass,
        ),
      ).toBeInTheDocument();
    }
  });

  it("renders loading, empty, and error chart states without fake marks", () => {
    const onRetry = vi.fn();
    const { rerender } = render(
      <AnalyticsCharts items={null} loading error={null} />,
    );
    expect(screen.getByText("Loading request charts…")).toBeInTheDocument();

    rerender(<AnalyticsCharts items={[]} loading={false} error={null} />);
    expect(
      screen.getByText("No request activity matches these filters."),
    ).toBeInTheDocument();
    expect(screen.queryByTestId(/request-log-volume-bar-/)).not.toBeInTheDocument();

    rerender(
      <AnalyticsCharts
        items={null}
        loading={false}
        error="Failed to load request aggregates"
      />,
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Failed to load request aggregates",
    );

    rerender(
      <AnalyticsCharts
        items={ITEMS}
        loading={false}
        error="Failed to refresh request aggregates"
        onRetry={onRetry}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry request charts" }));
    expect(onRetry).toHaveBeenCalledOnce();
    expect(screen.getByTestId("request-log-volume-chart")).toBeVisible();
  });

  it("shows the error and retry action when a refresh fails over retained empty data", () => {
    const onRetry = vi.fn();
    render(
      <AnalyticsCharts
        items={[]}
        loading={false}
        error="Failed to refresh request aggregates"
        onRetry={onRetry}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Failed to refresh request aggregates",
    );
    expect(
      screen.queryByText("No request activity matches these filters."),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Retry request charts" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("reduces oversized aggregate responses into a bounded number of marks", () => {
    const items = minuteBuckets(
      "2026-07-26T00:00:00Z",
      500,
      (minuteIndex) => minuteIndex + 1,
    );

    const reduced = reduceAggregateBuckets(items);

    // 500 minute buckets / 120 max marks => group size 5 => 100 marks.
    expect(reduced.groupSize).toBe(5);
    expect(reduced.buckets).toHaveLength(100);
    expect(reduced.buckets.length).toBeLessThanOrEqual(MAX_CHART_BUCKETS);
    expect(reduced.buckets[0]).toEqual({
      bucket: "2026-07-26T00:00:00.000Z",
      bucketEnd: "2026-07-26T00:04:00.000Z",
      sourceBucketCount: 5,
      count: 15, // 1 + 2 + 3 + 4 + 5
      status_2xx: 15,
      status_3xx: 0,
      status_4xx: 0,
      status_5xx: 0,
    });
    expect(reduced.buckets[99].count).toBe(2490); // 496 + 497 + 498 + 499 + 500
    expect(reduced.buckets[99].bucketEnd).toBe("2026-07-26T08:19:00.000Z");
  });

  it("records the actual source count for a short final folded group", () => {
    const reduced = reduceAggregateBuckets(
      minuteBuckets("2026-07-26T00:00:00Z", 7, () => 1),
      3,
    );

    expect(reduced.groupSize).toBe(3);
    expect(reduced.buckets.map((bucket) => bucket.sourceBucketCount)).toEqual([
      3, 3, 1,
    ]);
  });

  it("keeps buckets unreduced when the response fits the chart window", () => {
    const items = minuteBuckets("2026-07-26T00:00:00Z", MAX_CHART_BUCKETS, () => 2);

    const reduced = reduceAggregateBuckets(items);

    expect(reduced.groupSize).toBe(1);
    expect(reduced.buckets).toHaveLength(MAX_CHART_BUCKETS);
    expect(reduced.buckets[7].bucketEnd).toBe(reduced.buckets[7].bucket);
    expect(reduced.buckets[7].count).toBe(2);
  });

  it("renders only bounded marks and labels for an oversized aggregate response", () => {
    render(
      <AnalyticsCharts
        items={minuteBuckets(
          "2026-07-26T00:00:00Z",
          500,
          (minuteIndex) => minuteIndex + 1,
        )}
        loading={false}
        error={null}
      />,
    );

    const volumeChart = screen.getByTestId("request-log-volume-chart");
    const statusChart = screen.getByTestId("request-log-status-chart");
    expect(
      within(volumeChart).getAllByTestId(/^request-log-volume-bar-/),
    ).toHaveLength(100);
    expect(
      within(statusChart).getAllByTestId(/^request-log-status-2xx-/),
    ).toHaveLength(100);
    const visibleLabels = within(volumeChart).getAllByTestId(
      "request-log-bucket-label",
    );
    expect(visibleLabels.length).toBeGreaterThan(1);
    expect(visibleLabels.length).toBeLessThanOrEqual(12);
    for (const label of visibleLabels) {
      expect(label).not.toHaveTextContent("source bucket");
    }
    for (let index = 1; index < visibleLabels.length; index += 1) {
      const previous = visibleLabels[index - 1];
      const current = visibleLabels[index];
      const previousX = Number(previous.getAttribute("x"));
      const currentX = Number(current.getAttribute("x"));
      const widestLabelLength = Math.max(
        previous.textContent?.length ?? 0,
        current.textContent?.length ?? 0,
      );
      expect(currentX - previousX).toBeGreaterThanOrEqual(
        widestLabelLength * 6 + 8,
      );
    }
    expect(
      screen.getByTestId("request-log-chart-bucket-reduction"),
    ).toHaveTextContent(
      "Grouped 500 non-empty minute buckets into 100 marks; each mark represents up to 5 source buckets.",
    );
    expect(screen.getByTestId("request-log-volume-bar-0")).toHaveAttribute(
      "data-count",
      "15",
    );
    expect(screen.getByTestId("request-log-volume-bar-0")).toHaveTextContent(
      "5 source buckets (first 12:00 AM UTC; last 12:04 AM UTC): 15 requests",
    );
  });

  it("does not describe sparse folded buckets as continuous time ranges", () => {
    const items = minuteBuckets(
      "2026-07-26T00:00:00Z",
      121,
      () => 1,
    ).map((bucket, index) => ({
      ...bucket,
      bucket: new Date(
        new Date("2026-07-26T00:00:00Z").getTime() + index * 10 * MINUTE_MS,
      ).toISOString(),
    }));

    render(<AnalyticsCharts items={items} loading={false} error={null} />);

    expect(screen.getByTestId("request-log-volume-bar-0")).toHaveTextContent(
      "2 source buckets (first 12:00 AM UTC; last 12:10 AM UTC): 2 requests",
    );
    expect(
      screen.getByTestId("request-log-chart-bucket-reduction"),
    ).toHaveTextContent(
      "Grouped 121 non-empty minute buckets into 61 marks; each mark represents up to 2 source buckets.",
    );
    expect(screen.getByTestId("request-log-volume-bar-60")).toHaveTextContent(
      "8:00 PM UTC: 1 requests",
    );
  });

  it("keeps UTC dates in labels when buckets span multiple days", () => {
    const items: RequestLogAggregateBucket[] = [
      {
        bucket: "2026-07-26T23:59:00Z",
        count: 3,
        status_2xx: 3,
        status_3xx: 0,
        status_4xx: 0,
        status_5xx: 0,
      },
      {
        bucket: "2026-07-27T00:00:00Z",
        count: 5,
        status_2xx: 0,
        status_3xx: 0,
        status_4xx: 0,
        status_5xx: 5,
      },
    ];

    render(<AnalyticsCharts items={items} loading={false} error={null} />);

    const volumeChart = screen.getByTestId("request-log-volume-chart");
    const labels = within(volumeChart)
      .getAllByTestId("request-log-bucket-label")
      .map((label) => label.textContent);
    expect(labels).toEqual(["Jul 26, 11:59 PM UTC", "Jul 27, 12:00 AM UTC"]);
    expect(screen.getByTestId("request-log-volume-bar-1")).toHaveTextContent(
      "Jul 27, 12:00 AM UTC: 5 requests",
    );
    expect(screen.getByTestId("request-log-status-5xx-1")).toHaveTextContent(
      "Jul 27, 12:00 AM UTC 5xx: 5 requests",
    );
    expect(
      screen.queryByTestId("request-log-chart-bucket-reduction"),
    ).not.toBeInTheDocument();
  });

  it("omits the date from labels when all buckets share one UTC day", () => {
    render(<AnalyticsCharts items={ITEMS} loading={false} error={null} />);

    const labels = within(screen.getByTestId("request-log-volume-chart"))
      .getAllByTestId("request-log-bucket-label")
      .map((label) => label.textContent);
    expect(labels).toEqual(["12:00 PM UTC", "12:01 PM UTC"]);
  });
});
