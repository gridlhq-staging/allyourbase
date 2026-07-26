import type { ReactNode } from "react";
import type { RequestLogAggregateBucket } from "../types/analytics";

interface AnalyticsChartsProps {
  items: RequestLogAggregateBucket[] | null;
  loading: boolean;
  error: string | null;
  onRetry?: () => void;
}

const CHART_WIDTH = 600;
const CHART_VIEWBOX_HEIGHT = 170;
const CHART_BASELINE = 140;
const CHART_HEIGHT = 120;
const CHART_SIDE_PADDING = 32;
const CHART_LABEL_BASELINE = 160;
const MAX_BAR_WIDTH = 48;

/**
 * The aggregate endpoint returns every non-empty minute bucket in range, so the
 * charts fold the response into at most this many marks before rendering.
 */
export const MAX_CHART_BUCKETS = 120;

const MAX_VISIBLE_BUCKET_LABELS = 12;
const TICK_CHARACTER_WIDTH = 6;
const TICK_LABEL_GAP = 8;

const STATUS_CLASSES = [
  {
    key: "status_2xx",
    label: "2xx",
    colorClass: "text-emerald-500 dark:text-emerald-400",
    swatchClass: "bg-emerald-500 dark:bg-emerald-400",
  },
  {
    key: "status_3xx",
    label: "3xx",
    colorClass: "text-blue-500 dark:text-blue-400",
    swatchClass: "bg-blue-500 dark:bg-blue-400",
  },
  {
    key: "status_4xx",
    label: "4xx",
    colorClass: "text-amber-500 dark:text-amber-400",
    swatchClass: "bg-amber-500 dark:bg-amber-400",
  },
  {
    key: "status_5xx",
    label: "5xx",
    colorClass: "text-rose-500 dark:text-rose-400",
    swatchClass: "bg-rose-500 dark:bg-rose-400",
  },
] as const;

const TIME_LABEL_FORMATTER = new Intl.DateTimeFormat("en-US", {
  timeZone: "UTC",
  hour: "numeric",
  minute: "2-digit",
});

const DATE_TIME_LABEL_FORMATTER = new Intl.DateTimeFormat("en-US", {
  timeZone: "UTC",
  month: "short",
  day: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

/** One rendered mark: either a single minute bucket or a folded group of them. */
export interface ChartBucket extends RequestLogAggregateBucket {
  /** Last source bucket folded into this mark; equals `bucket` when unfolded. */
  bucketEnd: string;
  /** Number of response buckets represented by this mark. */
  sourceBucketCount: number;
}

export interface ReducedAggregateBuckets {
  buckets: ChartBucket[];
  /** Source minute buckets folded into each mark; 1 when nothing was folded. */
  groupSize: number;
}

function foldBucketGroup(group: RequestLogAggregateBucket[]): ChartBucket {
  const folded: ChartBucket = {
    bucket: group[0].bucket,
    bucketEnd: group[group.length - 1].bucket,
    sourceBucketCount: group.length,
    count: 0,
    status_2xx: 0,
    status_3xx: 0,
    status_4xx: 0,
    status_5xx: 0,
  };
  for (const source of group) {
    folded.count += source.count;
    folded.status_2xx += source.status_2xx;
    folded.status_3xx += source.status_3xx;
    folded.status_4xx += source.status_4xx;
    folded.status_5xx += source.status_5xx;
  }
  return folded;
}

/**
 * Folds adjacent response buckets into at most `maxBuckets` marks by summing
 * counts, so chart DOM size stays bounded no matter how much history matches.
 */
export function reduceAggregateBuckets(
  items: RequestLogAggregateBucket[],
  maxBuckets: number = MAX_CHART_BUCKETS,
): ReducedAggregateBuckets {
  const groupSize = Math.max(1, Math.ceil(items.length / maxBuckets));
  const buckets: ChartBucket[] = [];
  for (let start = 0; start < items.length; start += groupSize) {
    buckets.push(foldBucketGroup(items.slice(start, start + groupSize)));
  }
  return { buckets, groupSize };
}

function utcDayKey(bucket: string): string {
  const parsed = new Date(bucket);
  return Number.isNaN(parsed.getTime())
    ? bucket
    : parsed.toISOString().slice(0, 10);
}

function spansMultipleUtcDays(buckets: ChartBucket[]): boolean {
  const firstDay = utcDayKey(buckets[0].bucket);
  return buckets.some(
    (bucket) =>
      utcDayKey(bucket.bucket) !== firstDay ||
      utcDayKey(bucket.bucketEnd) !== firstDay,
  );
}

function accessibleBucketLabel(
  bucket: ChartBucket,
  formatter: Intl.DateTimeFormat,
): string {
  const start = new Date(bucket.bucket);
  if (Number.isNaN(start.getTime())) return bucket.bucket;
  const startLabel = formatter.format(start);
  if (bucket.sourceBucketCount === 1) return `${startLabel} UTC`;
  const end = new Date(bucket.bucketEnd);
  return Number.isNaN(end.getTime())
    ? `${bucket.sourceBucketCount.toLocaleString()} source buckets (first ${startLabel} UTC)`
    : `${bucket.sourceBucketCount.toLocaleString()} source buckets (first ${startLabel} UTC; last ${formatter.format(end)} UTC)`;
}

function visibleBucketLabel(bucket: ChartBucket, formatter: Intl.DateTimeFormat): string {
  const start = new Date(bucket.bucket);
  return Number.isNaN(start.getTime())
    ? bucket.bucket
    : `${formatter.format(start)} UTC`;
}

interface ChartLayout {
  buckets: ChartBucket[];
  maximum: number;
  slotWidth: number;
  barWidth: number;
  labelStride: number;
  labelFormatter: Intl.DateTimeFormat;
}

function chartLayout(buckets: ChartBucket[]): ChartLayout {
  const slotWidth = (CHART_WIDTH - CHART_SIDE_PADDING * 2) / buckets.length;
  const labelFormatter = spansMultipleUtcDays(buckets)
    ? DATE_TIME_LABEL_FORMATTER
    : TIME_LABEL_FORMATTER;
  const longestLabelLength = Math.max(...buckets.map(
    (bucket) => visibleBucketLabel(bucket, labelFormatter).length,
  ));
  const maximumLabelsByWidth = Math.max(
    1,
    Math.floor(
      (CHART_WIDTH - CHART_SIDE_PADDING * 2) /
        (longestLabelLength * TICK_CHARACTER_WIDTH + TICK_LABEL_GAP),
    ),
  );
  return {
    buckets,
    maximum: Math.max(...buckets.map((bucket) => bucket.count), 1),
    slotWidth,
    barWidth: Math.min(MAX_BAR_WIDTH, slotWidth * 0.7),
    labelStride: Math.ceil(
      buckets.length /
        Math.min(MAX_VISIBLE_BUCKET_LABELS, maximumLabelsByWidth),
    ),
    labelFormatter,
  };
}

function markX(layout: ChartLayout, index: number): number {
  return (
    CHART_SIDE_PADDING +
    index * layout.slotWidth +
    (layout.slotWidth - layout.barWidth) / 2
  );
}

function markHeight(layout: ChartLayout, value: number): number {
  return (value / layout.maximum) * CHART_HEIGHT;
}

function ChartRetryButton({ onRetry }: { onRetry?: () => void }) {
  if (!onRetry) return null;
  return (
    <button
      type="button"
      onClick={onRetry}
      className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
    >
      Retry request charts
    </button>
  );
}

function BucketLabels({ layout }: { layout: ChartLayout }) {
  return (
    <>
      {layout.buckets.map((bucket, index) =>
        index % layout.labelStride === 0 ? (
          <text
            key={bucket.bucket}
            data-testid="request-log-bucket-label"
            x={markX(layout, index) + layout.barWidth / 2}
            y={CHART_LABEL_BASELINE}
            textAnchor="middle"
            className="fill-gray-500 text-[10px] dark:fill-gray-300"
          >
            {visibleBucketLabel(bucket, layout.labelFormatter)}
          </text>
        ) : null,
      )}
    </>
  );
}

function ChartFrame({
  testId,
  titleId,
  title,
  layout,
  children,
}: {
  testId: string;
  titleId: string;
  title: string;
  layout: ChartLayout;
  children: ReactNode;
}) {
  return (
    <svg
      data-testid={testId}
      viewBox={`0 0 ${CHART_WIDTH} ${CHART_VIEWBOX_HEIGHT}`}
      className="h-48 w-full"
      role="img"
      aria-labelledby={titleId}
    >
      <title id={titleId}>{title}</title>
      <line
        x1={CHART_SIDE_PADDING}
        y1={CHART_BASELINE}
        x2={CHART_WIDTH - CHART_SIDE_PADDING}
        y2={CHART_BASELINE}
        stroke="currentColor"
        className="text-gray-300 dark:text-gray-700"
      />
      {children}
      <BucketLabels layout={layout} />
    </svg>
  );
}

function VolumeChart({ layout }: { layout: ChartLayout }) {
  return (
    <ChartFrame
      testId="request-log-volume-chart"
      titleId="request-log-volume-title"
      title="Request volume by minute"
      layout={layout}
    >
      {layout.buckets.map((bucket, index) => {
        const height = markHeight(layout, bucket.count);
        return (
          <rect
            key={bucket.bucket}
            data-testid={`request-log-volume-bar-${index}`}
            data-bucket={bucket.bucket}
            data-count={bucket.count}
            x={markX(layout, index)}
            y={CHART_BASELINE - height}
            width={layout.barWidth}
            height={height}
            rx="3"
            fill="currentColor"
            className="text-indigo-500 dark:text-indigo-400"
          >
            <title>
              {accessibleBucketLabel(bucket, layout.labelFormatter)}:{" "}
              {bucket.count.toLocaleString()} requests
            </title>
          </rect>
        );
      })}
    </ChartFrame>
  );
}

function StatusStack({
  layout,
  bucket,
  index,
}: {
  layout: ChartLayout;
  bucket: ChartBucket;
  index: number;
}) {
  const x = markX(layout, index);
  let segmentBottom = CHART_BASELINE;
  return (
    <g>
      {STATUS_CLASSES.map((statusClass) => {
        const count = bucket[statusClass.key];
        const height = markHeight(layout, count);
        segmentBottom -= height;
        return (
          <rect
            key={statusClass.key}
            data-testid={`request-log-status-${statusClass.label}-${index}`}
            data-bucket={bucket.bucket}
            data-count={count}
            x={x}
            y={segmentBottom}
            width={layout.barWidth}
            height={height}
            fill="currentColor"
            className={statusClass.colorClass}
          >
            <title>
              {accessibleBucketLabel(bucket, layout.labelFormatter)}{" "}
              {statusClass.label}:{" "}
              {count.toLocaleString()} requests
            </title>
          </rect>
        );
      })}
    </g>
  );
}

function StatusChart({ layout }: { layout: ChartLayout }) {
  return (
    <ChartFrame
      testId="request-log-status-chart"
      titleId="request-log-status-title"
      title="Request status classes by minute"
      layout={layout}
    >
      {layout.buckets.map((bucket, index) => (
        <StatusStack
          key={bucket.bucket}
          layout={layout}
          bucket={bucket}
          index={index}
        />
      ))}
    </ChartFrame>
  );
}

function StatusLegend() {
  return (
    <ul
      data-testid="request-log-status-legend"
      className="flex flex-wrap gap-3 text-xs text-gray-600 dark:text-gray-300"
    >
      {STATUS_CLASSES.map((statusClass) => (
        <li key={statusClass.key} className="flex items-center gap-1">
          <span
            aria-hidden="true"
            className={`h-2.5 w-2.5 rounded-sm ${statusClass.swatchClass}`}
          />
          {statusClass.label}
        </li>
      ))}
    </ul>
  );
}

function BucketReductionNote({
  sourceBucketCount,
  reduced,
}: {
  sourceBucketCount: number;
  reduced: ReducedAggregateBuckets;
}) {
  if (reduced.groupSize === 1) return null;
  return (
    <p
      data-testid="request-log-chart-bucket-reduction"
      className="text-xs text-gray-500 dark:text-gray-400 lg:col-span-2"
    >
      Grouped {sourceBucketCount.toLocaleString()} non-empty minute buckets into{" "}
      {reduced.buckets.length.toLocaleString()} marks; each mark represents up
      to {reduced.groupSize.toLocaleString()} source buckets.
    </p>
  );
}

/**
 * Renders the no-data chart states. An error outranks both loading and the
 * empty state so a failed refresh over empty data still offers a retry instead
 * of reporting no activity.
 */
function ChartsPlaceholder({
  loading,
  error,
  onRetry,
}: {
  loading: boolean;
  error: string | null;
  onRetry?: () => void;
}) {
  if (error) {
    return (
      <section data-testid="request-log-aggregate-charts">
        <p role="alert" className="text-sm text-red-600 dark:text-red-400">
          {error}
        </p>
        <div className="mt-2">
          <ChartRetryButton onRetry={onRetry} />
        </div>
      </section>
    );
  }

  return (
    <section data-testid="request-log-aggregate-charts">
      <p className="text-sm text-gray-500 dark:text-gray-300">
        {loading
          ? "Loading request charts…"
          : "No request activity matches these filters."}
      </p>
    </section>
  );
}

export function AnalyticsCharts({
  items,
  loading,
  error,
  onRetry,
}: AnalyticsChartsProps) {
  if (items === null || items.length === 0) {
    return (
      <ChartsPlaceholder loading={loading} error={error} onRetry={onRetry} />
    );
  }

  const reduced = reduceAggregateBuckets(items);
  const layout = chartLayout(reduced.buckets);

  return (
    <section
      data-testid="request-log-aggregate-charts"
      aria-label="Request log charts"
      aria-busy={loading}
      className="mb-5 grid gap-4 lg:grid-cols-2"
    >
      {error && (
        <div className="rounded border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-950/30 dark:text-red-300 lg:col-span-2">
          <p role="alert">{error}</p>
          <ChartRetryButton onRetry={onRetry} />
        </div>
      )}
      <div className="rounded border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900">
        <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
          Request volume
        </h2>
        <VolumeChart layout={layout} />
      </div>
      <div className="rounded border border-gray-200 bg-white p-3 dark:border-gray-700 dark:bg-gray-900">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-sm font-semibold text-gray-900 dark:text-gray-100">
            Status classes
          </h2>
          <StatusLegend />
        </div>
        <StatusChart layout={layout} />
      </div>
      <BucketReductionNote
        sourceBucketCount={items.length}
        reduced={reduced}
      />
    </section>
  );
}
