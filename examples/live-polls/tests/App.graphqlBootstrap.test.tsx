import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import App from "../src/App";
import { ayb } from "../src/lib/ayb";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";

const SEED_GUARD_KEY = "ayb_live_polls_bootstrap_seeded";

vi.mock("../src/components/AuthForm", () => ({
  default: () => <button type="button">Auth Form</button>,
}));

vi.mock("../src/components/CreatePoll", () => ({
  default: () => null,
}));

vi.mock("../src/components/PollCard", () => ({
  default: ({ poll, options, votes }: { poll: { question: string }; options: unknown[]; votes: unknown[] }) => (
    <article data-testid="poll-card">
      <h2>{poll.question}</h2>
      <span data-testid="options-count">{options.length}</span>
      <span data-testid="votes-count">{votes.length}</span>
    </article>
  ),
}));

vi.mock("../src/hooks/useRealtime", () => ({
  useRealtime: vi.fn(),
}));

vi.mock("../src/lib/ayb", () => ({
  ayb: {
    graphql: {
      query: vi.fn().mockResolvedValue({ polls: [] }),
    },
    records: {
      list: vi.fn().mockResolvedValue({ items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 }),
      create: vi.fn(),
    },
  },
  clearPersistedTokens: vi.fn(),
  isAnonymousBootstrapEnabled: vi.fn(() => true),
  disableAnonymousBootstrap: vi.fn(),
  clearAnonymousBootstrapOptOut: vi.fn(),
  hasLivePollsBootstrapSeeded: vi.fn(() => sessionStorage.getItem(SEED_GUARD_KEY) === "1"),
  markLivePollsBootstrapSeeded: vi.fn(() => sessionStorage.setItem(SEED_GUARD_KEY, "1")),
  clearLivePollsBootstrapSeeded: vi.fn(() => sessionStorage.removeItem(SEED_GUARD_KEY)),
}));

vi.mock("@allyourbase/react", () => ({
  useAuth: vi.fn(),
  useAybAnonymousBootstrap: vi.fn(),
}));

const mockUseAuth = vi.mocked(useAuth);
const mockUseAybAnonymousBootstrap = vi.mocked(useAybAnonymousBootstrap);
const mockGraphQLQuery = vi.mocked(ayb.graphql.query);
const mockListRecords = vi.mocked(ayb.records.list);
const mockCreateRecord = vi.mocked(ayb.records.create);

describe("App GraphQL bootstrap fallback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    sessionStorage.removeItem(SEED_GUARD_KEY);

    mockUseAuth.mockReturnValue({
      loading: false,
      user: { id: "user-seed", email: "seed@test.com", isAnonymous: false },
      error: null,
      token: "token-seed",
      refreshToken: "refresh-seed",
      login: vi.fn(),
      register: vi.fn(),
      signInAnonymously: vi.fn(),
      linkEmail: vi.fn(),
      signInWithOAuth: vi.fn(),
      logout: vi.fn().mockResolvedValue(undefined),
      refresh: vi.fn(),
    });
    mockUseAybAnonymousBootstrap.mockReturnValue({ bootstrapping: false });
    mockGraphQLQuery.mockResolvedValue({ polls: [] });
    mockListRecords.mockResolvedValue({ items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 });

    mockCreateRecord.mockImplementation(async (table: string, payload: Record<string, unknown>) => {
      if (table === "polls") {
        return {
          id: "seed-poll-1",
          user_id: payload.user_id,
          question: payload.question,
          is_closed: false,
          created_at: "2026-01-01T00:00:00Z",
        };
      }

      if (table === "poll_options") {
        return {
          id: `${String(payload.poll_id)}-${String(payload.position)}`,
          poll_id: payload.poll_id,
          label: payload.label,
          position: payload.position,
        };
      }

      if (table === "votes") {
        return {
          id: "seed-vote-1",
          poll_id: payload.poll_id,
          option_id: payload.option_id,
          user_id: payload.user_id,
          created_at: "2026-01-01T00:00:00Z",
        };
      }

      throw new Error(`Unexpected table: ${table}`);
    });
  });

  it("seeds empty GraphQL bootstrap only once per browser session", async () => {
    const firstRender = render(<App />);

    await waitFor(() => expect(mockGraphQLQuery).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(mockListRecords).toHaveBeenCalledWith("votes", expect.objectContaining({ page: 1, perPage: 500 })));

    expect(await screen.findByTestId("poll-card")).toBeInTheDocument();

    firstRender.unmount();
    render(<App />);

    await waitFor(() => expect(mockGraphQLQuery).toHaveBeenCalledTimes(2));

    const pollsCreates = mockCreateRecord.mock.calls.filter(([table]) => table === "polls");
    const optionCreates = mockCreateRecord.mock.calls.filter(([table]) => table === "poll_options");
    const voteCreates = mockCreateRecord.mock.calls.filter(([table]) => table === "votes");

    expect(pollsCreates).toHaveLength(1);
    expect(optionCreates.length).toBeGreaterThanOrEqual(2);
    expect(voteCreates).toHaveLength(1);
  });

  it("retries seeding after unmounting before first seed completes", async () => {
    let firstPollCreatePending = true;
    mockCreateRecord.mockImplementation(async (table: string, payload: Record<string, unknown>) => {
      if (table === "polls" && firstPollCreatePending) {
        firstPollCreatePending = false;
        return new Promise(() => undefined);
      }

      if (table === "polls") {
        return {
          id: "seed-poll-2",
          user_id: payload.user_id,
          question: payload.question,
          is_closed: false,
          created_at: "2026-01-01T00:00:00Z",
        };
      }

      if (table === "poll_options") {
        return {
          id: `${String(payload.poll_id)}-${String(payload.position)}`,
          poll_id: payload.poll_id,
          label: payload.label,
          position: payload.position,
        };
      }

      if (table === "votes") {
        return {
          id: "seed-vote-2",
          poll_id: payload.poll_id,
          option_id: payload.option_id,
          user_id: payload.user_id,
          created_at: "2026-01-01T00:00:00Z",
        };
      }

      throw new Error(`Unexpected table: ${table}`);
    });

    const firstRender = render(<App />);
    await waitFor(() => expect(mockCreateRecord).toHaveBeenCalledWith("polls", expect.any(Object)));
    firstRender.unmount();

    render(<App />);

    await waitFor(() => {
      const pollCreates = mockCreateRecord.mock.calls.filter(([table]) => table === "polls");
      expect(pollCreates).toHaveLength(2);
    });
  });

  it("falls back to records.list when graphql bootstrap fails", async () => {
    mockGraphQLQuery.mockRejectedValueOnce(new Error("graphql route unavailable"));
    mockListRecords.mockImplementation(async (table: string) => {
      if (table === "polls") {
        return {
          items: [
            {
              id: "poll-rest-1",
              user_id: "user-seed",
              question: "Poll from REST fallback?",
              is_closed: false,
              created_at: "2026-01-01T00:00:00Z",
              poll_options: [
                { id: "opt-rest-1", poll_id: "poll-rest-1", label: "Yes", position: 0 },
                { id: "opt-rest-2", poll_id: "poll-rest-1", label: "No", position: 1 },
              ],
            },
          ],
          page: 1,
          perPage: 100,
          totalItems: 1,
          totalPages: 1,
        };
      }
      if (table === "votes") {
        return { items: [], page: 1, perPage: 500, totalItems: 0, totalPages: 0 };
      }
      return { items: [], page: 1, perPage: 100, totalItems: 0, totalPages: 0 };
    });

    render(<App />);

    expect(await screen.findByText("Poll from REST fallback?")).toBeInTheDocument();
    expect(screen.queryByText("Could not load polls. Please refresh.")).not.toBeInTheDocument();
    expect(mockListRecords).toHaveBeenCalledWith(
      "polls",
      expect.objectContaining({
        page: 1,
        perPage: 100,
        sort: "-created_at",
        expand: "poll_options",
      }),
    );
  });
});
