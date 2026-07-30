import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { SecurityAdvisorReport } from "../../types";
import { SecurityAdvisor } from "../SecurityAdvisor";
import { toPanelError } from "../advisors/panelError";

vi.mock("../../api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../api")>();
  return { ...actual, getSecurityAdvisorReport: vi.fn() };
});

import { ApiError, getSecurityAdvisorReport } from "../../api";

const mockedGet = vi.mocked(getSecurityAdvisorReport);

function makeReport(): SecurityAdvisorReport {
  return {
    evaluatedAt: "2026-02-28T00:00:00Z",
    stale: false,
    findings: [
      {
        id: "f1",
        severity: "critical",
        category: "rls",
        status: "open",
        title: "RLS disabled on public.posts",
        description: "Table is readable by broad roles",
        remediation: "Enable RLS and add restrictive policies",
      },
      {
        id: "f2",
        severity: "low",
        category: "auth",
        status: "open",
        title: "Email verification optional",
        description: "Optional verification can increase abuse risk",
        remediation: "Require email verification",
      },
    ],
  };
}

describe("SecurityAdvisor", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedGet.mockResolvedValue(makeReport());
  });

  it("groups findings by severity and shows details", async () => {
    const user = userEvent.setup();
    render(<SecurityAdvisor />);

    // Await a finding, not the panel heading: the heading is part of the static
    // shell that renders before the report resolves, so waiting on it would let
    // the synchronous finding assertions below race the fetch.
    await screen.findByText(/RLS disabled on public.posts/i);
    expect(screen.getByRole("heading", { name: /critical/i })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /RLS disabled on public.posts/i }));
    expect(screen.getByText(/Enable RLS and add restrictive policies/i)).toBeInTheDocument();
  });

  it("filters findings by severity", async () => {
    const user = userEvent.setup();
    render(<SecurityAdvisor />);

    // Await a finding so the severity filter runs against a populated report.
    await screen.findByText(/RLS disabled on public.posts/i);
    await user.selectOptions(screen.getByLabelText(/Severity/i), "critical");

    expect(screen.getByText(/RLS disabled on public.posts/i)).toBeInTheDocument();
    expect(screen.queryByText(/Email verification optional/i)).not.toBeInTheDocument();
  });

  it("shows stale data warning", async () => {
    mockedGet.mockResolvedValueOnce({ ...makeReport(), stale: true });
    render(<SecurityAdvisor />);

    await screen.findByText(/Telemetry may be stale/i);
  });

  it("shows shared panel error copy when the security report request fails", async () => {
    const apiError = new ApiError(500, "telemetry backend unavailable");
    mockedGet.mockRejectedValueOnce(apiError);
    render(<SecurityAdvisor />);

    await screen.findByText(toPanelError(apiError));
  });

  it("recovers a failed security report through an error-scoped Retry that refetches", async () => {
    const user = userEvent.setup();
    const apiError = new ApiError(500, "telemetry backend unavailable");
    mockedGet.mockRejectedValueOnce(apiError);
    render(<SecurityAdvisor />);

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(toPanelError(apiError));
    // Panel shell (heading + filters) stays mounted alongside the error.
    expect(screen.getByRole("heading", { name: "Security Advisor" })).toBeInTheDocument();
    expect(screen.getByLabelText(/Severity/i)).toBeInTheDocument();

    const callsBeforeRetry = mockedGet.mock.calls.length;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() => expect(mockedGet.mock.calls.length).toBeGreaterThan(callsBeforeRetry));
    await screen.findByText(/RLS disabled on public.posts/i);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});
