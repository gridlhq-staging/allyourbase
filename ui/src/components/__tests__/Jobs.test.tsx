import { vi, describe, it, expect, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { Jobs } from "../Jobs";
import {
  listJobs,
  listJobRuns,
  getQueueStats,
  retryJob,
  cancelJob,
} from "../../api";
import type {
  JobListResponse,
  JobResponse,
  JobRunListResponse,
  JobRunResponse,
  QueueStats,
} from "../../types";

vi.mock("../../api", () => ({
  listJobs: vi.fn(),
  listJobRuns: vi.fn(),
  getQueueStats: vi.fn(),
  retryJob: vi.fn(),
  cancelJob: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListJobs = vi.mocked(listJobs);
const mockListJobRuns = vi.mocked(listJobRuns);
const mockGetQueueStats = vi.mocked(getQueueStats);
const mockRetryJob = vi.mocked(retryJob);
const mockCancelJob = vi.mocked(cancelJob);

function makeJob(overrides: Partial<JobResponse> = {}): JobResponse {
  return {
    id: "j1",
    type: "webhook_delivery_prune",
    payload: {},
    state: "queued",
    runAt: "2026-02-22T10:00:00Z",
    leaseUntil: null,
    workerId: null,
    attempts: 0,
    maxAttempts: 3,
    lastError: null,
    lastRunAt: null,
    idempotencyKey: null,
    scheduleId: null,
    createdAt: "2026-02-22T09:00:00Z",
    updatedAt: "2026-02-22T09:00:00Z",
    completedAt: null,
    canceledAt: null,
    ...overrides,
  };
}

function makeListResponse(items: JobResponse[]): JobListResponse {
  return { items, count: items.length };
}

function makeStats(overrides: Partial<QueueStats> = {}): QueueStats {
  return {
    queued: 1,
    running: 0,
    completed: 0,
    failed: 0,
    canceled: 0,
    oldestQueuedAgeSec: 12,
    ...overrides,
  };
}

function makeRun(overrides: Partial<JobRunResponse> = {}): JobRunResponse {
  return {
    attempt: 1,
    status: "completed",
    startedAt: "2026-02-22T09:02:00Z",
    finishedAt: "2026-02-22T09:02:05Z",
    durationMs: 5000,
    ...overrides,
  };
}

function makeRunListResponse(items: JobRunResponse[]): JobRunListResponse {
  return { items, count: items.length };
}

function makeDeferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("Jobs", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListJobRuns.mockResolvedValue(makeRunListResponse([]));
    mockGetQueueStats.mockResolvedValue(makeStats());
    mockRetryJob.mockResolvedValue(makeJob({ state: "queued" }));
    mockCancelJob.mockResolvedValue(makeJob({ state: "canceled" }));
  });

  it("shows loading state", () => {
    mockListJobs.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<Jobs />);
    expect(screen.getByText("Loading jobs...")).toBeInTheDocument();
  });

  it("renders jobs table and queue stats", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([
        makeJob({ id: "j1", state: "failed", lastError: "boom" }),
        makeJob({ id: "j2", state: "queued" }),
      ]),
    );

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("Job Queue")).toBeInTheDocument();
      expect(screen.getAllByText("failed").length).toBeGreaterThanOrEqual(1);
      expect(screen.getAllByText("queued").length).toBeGreaterThanOrEqual(1);
      expect(screen.getByText("boom")).toBeInTheDocument();
      expect(screen.getByText("Queued: 1")).toBeInTheDocument();
    });
  });

  it("keeps rendering jobs when queue stats fails", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([makeJob({ id: "j1", state: "failed", lastError: "boom" })]),
    );
    mockGetQueueStats.mockRejectedValueOnce(new Error("stats unavailable"));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("Job Queue")).toBeInTheDocument();
      expect(screen.getByText("boom")).toBeInTheDocument();
    });

    expect(screen.queryByText("Queued: 1")).not.toBeInTheDocument();
    expect(screen.queryByText("stats unavailable")).not.toBeInTheDocument();
  });

  it("applies state and type filters", async () => {
    mockListJobs.mockResolvedValue(makeListResponse([makeJob()]));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenCalledWith({});
    });

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("State"), "failed");
    await user.type(screen.getByLabelText("Type"), "webhook");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenLastCalledWith({
        state: "failed",
        type: "webhook",
      });
    });
  });

  it("shows first-visit empty state when queue has no jobs", async () => {
    mockListJobs.mockResolvedValueOnce(makeListResponse([]));
    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("No jobs in queue yet")).toBeInTheDocument();
      expect(
        screen.getByText(
          "Run a background task, webhook delivery, or scheduled job, then refresh to see it here.",
        ),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Refresh jobs" }),
      ).toBeInTheDocument();
    });
  });

  it("shows filtered empty state and clear filters action", async () => {
    mockListJobs.mockResolvedValue(makeListResponse([]));
    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(mockListJobs).toHaveBeenCalledWith({});
    });

    await user.selectOptions(screen.getByLabelText("State"), "failed");
    await user.type(screen.getByLabelText("Type"), "webhook");
    await user.click(screen.getByRole("button", { name: "Apply Filters" }));

    await waitFor(() => {
      expect(screen.getByText("No jobs match these filters")).toBeInTheDocument();
      expect(
        screen.getByText("Clear filters to see all jobs, or adjust state and type."),
      ).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Clear filters" }),
      ).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Clear filters" }));

    await waitFor(() => {
      expect(mockListJobs).toHaveBeenLastCalledWith({});
    });
  });

  it("retries a failed job", async () => {
    mockListJobs.mockResolvedValue(
      makeListResponse([makeJob({ id: "j-fail", state: "failed" })]),
    );

    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Retry job j-fail")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Retry job j-fail"));

    await waitFor(() => {
      expect(mockRetryJob).toHaveBeenCalledWith("j-fail");
    });
  });

  it("cancels a queued job", async () => {
    mockListJobs.mockResolvedValue(
      makeListResponse([makeJob({ id: "j-queued", state: "queued" })]),
    );

    renderWithProviders(<Jobs />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByLabelText("Cancel job j-queued")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Cancel job j-queued"));

    await waitFor(() => {
      expect(mockCancelJob).toHaveBeenCalledWith("j-queued");
    });
  });

  it("shows page error when jobs request fails even if stats succeed", async () => {
    mockListJobs.mockRejectedValueOnce(new Error("jobs unavailable"));

    renderWithProviders(<Jobs />);

    await waitFor(() => {
      expect(screen.getByText("jobs unavailable")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("opens a run-history view and renders exact run values", async () => {
    mockListJobs.mockResolvedValueOnce(makeListResponse([makeJob({ id: "j1", state: "failed" })]));
    const runsDeferred = makeDeferred<JobRunListResponse>();
    mockListJobRuns.mockReturnValueOnce(runsDeferred.promise);

    renderWithProviders(<Jobs />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByLabelText("View runs for job j1")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("View runs for job j1"));

    expect(mockListJobRuns).toHaveBeenCalledWith("j1");
    expect(screen.getByText("Loading run history...")).toBeInTheDocument();

    runsDeferred.resolve(
      makeRunListResponse([
        makeRun({ attempt: 1, status: "failed", durationMs: 2100, error: "first failure" }),
        makeRun({ attempt: 2, status: "completed", durationMs: 975, error: undefined }),
      ]),
    );

    await waitFor(() => {
      expect(screen.getByText("Run history for job j1")).toBeInTheDocument();
      expect(screen.getByText("2100 ms")).toBeInTheDocument();
      expect(screen.getByText("975 ms")).toBeInTheDocument();
      expect(screen.getByText("first failure")).toBeInTheDocument();
      expect(screen.getAllByText("completed").length).toBeGreaterThanOrEqual(1);
    });
  });

  it("shows run-history empty state", async () => {
    mockListJobs.mockResolvedValueOnce(makeListResponse([makeJob({ id: "j1" })]));
    mockListJobRuns.mockResolvedValueOnce(makeRunListResponse([]));

    renderWithProviders(<Jobs />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByLabelText("View runs for job j1")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("View runs for job j1"));

    await waitFor(() => {
      expect(screen.getByText("No run history found for this job.")).toBeInTheDocument();
    });
  });

  it("shows run-history error state and retries fetch", async () => {
    mockListJobs.mockResolvedValueOnce(makeListResponse([makeJob({ id: "j1" })]));
    mockListJobRuns
      .mockRejectedValueOnce(new Error("runs unavailable"))
      .mockResolvedValueOnce(makeRunListResponse([makeRun({ attempt: 1, status: "completed" })]));

    renderWithProviders(<Jobs />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByLabelText("View runs for job j1")).toBeInTheDocument();
    });
    await user.click(screen.getByLabelText("View runs for job j1"));

    await waitFor(() => {
      expect(screen.getByText("runs unavailable")).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Retry run history" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Retry run history" }));

    await waitFor(() => {
      expect(mockListJobRuns).toHaveBeenCalledTimes(2);
      expect(screen.getByText("Run history for job j1")).toBeInTheDocument();
    });
  });

  it("clears stale run history while switching and closing selection", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([makeJob({ id: "j1" }), makeJob({ id: "j2" })]),
    );

    const secondRunsDeferred = makeDeferred<JobRunListResponse>();
    mockListJobRuns
      .mockResolvedValueOnce(makeRunListResponse([makeRun({ error: "stale failure" })]))
      .mockReturnValueOnce(secondRunsDeferred.promise);

    renderWithProviders(<Jobs />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByLabelText("View runs for job j1")).toBeInTheDocument();
      expect(screen.getByLabelText("View runs for job j2")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("View runs for job j1"));
    await waitFor(() => {
      expect(screen.getByText("stale failure")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("View runs for job j2"));

    expect(screen.queryByText("stale failure")).not.toBeInTheDocument();
    expect(screen.getByText("Loading run history...")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Close run history" }));
    expect(screen.queryByText("Run history for job j2")).not.toBeInTheDocument();

    secondRunsDeferred.resolve(makeRunListResponse([makeRun({ error: "next failure" })]));
    await waitFor(() => {
      expect(screen.queryByText("next failure")).not.toBeInTheDocument();
    });
  });

  it("keeps retry and cancel actions functional while run history is visible", async () => {
    mockListJobs.mockResolvedValueOnce(
      makeListResponse([
        makeJob({ id: "j-fail", state: "failed" }),
        makeJob({ id: "j-queued", state: "queued" }),
      ]),
    );
    mockListJobRuns.mockResolvedValueOnce(makeRunListResponse([makeRun()]));

    renderWithProviders(<Jobs />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByLabelText("View runs for job j-fail")).toBeInTheDocument();
    });
    await user.click(screen.getByLabelText("View runs for job j-fail"));

    await waitFor(() => {
      expect(screen.getByText("Run history for job j-fail")).toBeInTheDocument();
    });

    await user.click(screen.getByLabelText("Retry job j-fail"));
    await user.click(screen.getByLabelText("Cancel job j-queued"));

    await waitFor(() => {
      expect(mockRetryJob).toHaveBeenCalledWith("j-fail");
      expect(mockCancelJob).toHaveBeenCalledWith("j-queued");
    });
  });
});
