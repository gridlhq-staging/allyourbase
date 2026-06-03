import { useCallback, useEffect, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import AuthForm from "./components/AuthForm";
import CreatePoll from "./components/CreatePoll";
import PollCard from "./components/PollCard";
import { useRealtime, type RealtimeEvent } from "./hooks/useRealtime";
import {
  ayb,
  clearLivePollsBootstrapSeeded,
  clearAnonymousBootstrapOptOut,
  clearPersistedTokens,
  disableAnonymousBootstrap,
  hasLivePollsBootstrapSeeded,
  isAnonymousBootstrapEnabled,
  markLivePollsBootstrapSeeded,
} from "./lib/ayb";
import { castVoteRecord, createPollWithOptions } from "./lib/recordsWriteContracts";
import type { Poll, PollOption, Vote } from "./types";

type PollWithOptions = Poll & { poll_options?: PollOption[] };
type SeededBootstrapData = {
  poll: PollWithOptions;
  vote: Vote;
};

function normalizeRealtimeAction(
  action: RealtimeEvent["action"],
): "create" | "update" | "delete" | null {
  const normalized = action.toLowerCase();
  if (normalized === "create" || normalized === "insert") {
    return "create";
  }
  if (normalized === "update") {
    return "update";
  }
  if (normalized === "delete") {
    return "delete";
  }
  return null;
}

const SEEDED_POLL_QUESTION = "How do you like this live polls demo?";
const SEEDED_OPTION_LABELS = ["Looks great", "Needs tweaks"] as const;

async function fetchAll<T>(table: string, opts: Record<string, unknown> = {}): Promise<T[]> {
  const pageSize = 500;
  const items: T[] = [];
  let page = 1;
  for (;;) {
    const res = await ayb.records.list<T>(table, { ...opts, page, perPage: pageSize });
    items.push(...res.items);
    if (res.items.length < pageSize) break;
    page++;
  }
  return items;
}

async function loadBootstrapPolls(): Promise<PollWithOptions[]> {
  try {
    const pollData = await ayb.graphql.query<{ polls: PollWithOptions[] }>(`
      query LivePollsBootstrap {
        polls(limit: 100, orderBy: [{ created_at: desc }]) {
          id
          user_id
          question
          is_closed
          created_at
          poll_options {
            id
            poll_id
            label
            position
          }
        }
      }
    `);
    return pollData.polls;
  } catch {
    // Stage 5 still verifies GraphQL request bootstrap at the E2E layer, but
    // the runtime should not drop user-visible polls when GraphQL is missing.
    const fallback = await ayb.records.list<PollWithOptions>("polls", {
      page: 1,
      perPage: 100,
      sort: "-created_at",
      expand: "poll_options",
    });
    return fallback.items;
  }
}

function buildOptionMap(polls: PollWithOptions[]): Map<string, PollOption[]> {
  const optionMap = new Map<string, PollOption[]>();
  for (const poll of polls) {
    const sortedOptions = [...(poll.poll_options ?? [])].sort((a, b) => a.position - b.position);
    optionMap.set(poll.id, sortedOptions);
  }
  return optionMap;
}

function buildVoteMap(votes: Vote[]): Map<string, Vote[]> {
  const voteMap = new Map<string, Vote[]>();
  for (const vote of votes) {
    const pollVotes = voteMap.get(vote.poll_id) ?? [];
    pollVotes.push(vote);
    voteMap.set(vote.poll_id, pollVotes);
  }
  return voteMap;
}

async function seedBootstrapPoll(userId: string): Promise<SeededBootstrapData> {
  const { poll, options: pollOptions } = await createPollWithOptions({
    question: SEEDED_POLL_QUESTION,
    userId,
    optionLabels: [...SEEDED_OPTION_LABELS],
  });

  const vote = await castVoteRecord({
    pollId: poll.id,
    optionId: pollOptions[0].id,
    userId,
  });

  return {
    poll: { ...poll, poll_options: pollOptions },
    vote,
  };
}

export default function App() {
  const { user, token, loading, logout, signInAnonymously } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  // Pass the resolved token and auth method from useAuth so bootstrap does not
  // race explicit sign-in or registration with a fresh anonymous session.
  const { bootstrapping } = useAybAnonymousBootstrap({
    enabled: anonymousBootstrapEnabled,
    token,
    signInAnonymously,
  });
  const authed = Boolean(token) && user != null;
  const canCreatePolls = authed && user != null && !user.isAnonymous;
  const [userId, setUserId] = useState<string | null>(null);
  const [userEmail, setUserEmail] = useState<string | null>(null);
  const [polls, setPolls] = useState<Poll[]>([]);
  const [optionsMap, setOptionsMap] = useState<Map<string, PollOption[]>>(new Map());
  const [votesMap, setVotesMap] = useState<Map<string, Vote[]>>(new Map());
  const [showCreate, setShowCreate] = useState(false);
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const hydratePollOptions = useCallback(async (pollId: string) => {
    try {
      const allOptions = await fetchAll<PollOption>("poll_options", {
        sort: "position",
      });
      const hydrated = allOptions
        .filter((option) => option.poll_id === pollId)
        .sort((a, b) => a.position - b.position);
      if (hydrated.length === 0) {
        return;
      }
      setOptionsMap((prev) => {
        const existing = prev.get(pollId) ?? [];
        if (existing.length >= hydrated.length) {
          return prev;
        }
        const next = new Map(prev);
        next.set(pollId, hydrated);
        return next;
      });
    } catch {
      // Best effort: realtime poll rows should still render even if option
      // hydration fails in the background.
    }
  }, []);

  useEffect(() => {
    if (!authed || !user) {
      setUserId(null);
      setUserEmail(null);
      return;
    }
    setUserId(user.id);
    setUserEmail(user.email ?? null);
  }, [authed, user]);

  useEffect(() => {
    if (!authed) {
      setLoadError(null);
      return;
    }
    async function load() {
      try {
        const [polls, allVotes] = await Promise.all([
          loadBootstrapPolls(),
          fetchAll<Vote>("votes"),
        ]);
        let bootstrapPolls = polls;
        let bootstrapVotes = allVotes;

        const shouldSeed = bootstrapPolls.length === 0 && bootstrapVotes.length === 0 && user != null;
        if (shouldSeed && !hasLivePollsBootstrapSeeded()) {
          try {
            const seededData = await seedBootstrapPoll(user.id);
            markLivePollsBootstrapSeeded();
            bootstrapPolls = [seededData.poll];
            bootstrapVotes = [seededData.vote];
          } catch (error) {
            clearLivePollsBootstrapSeeded();
            throw error;
          }
        }

        setPolls(bootstrapPolls);

        const optionMap = buildOptionMap(bootstrapPolls);
        setOptionsMap((prev) => {
          const next = new Map(prev);
          for (const [pollId, pollOptions] of optionMap) {
            next.set(pollId, pollOptions);
          }
          return next;
        });
        for (const poll of bootstrapPolls) {
          if ((poll.poll_options ?? []).length === 0) {
            void hydratePollOptions(poll.id);
          }
        }

        const voteMap = buildVoteMap(bootstrapVotes);
        setVotesMap((prev) => {
          const next = new Map(prev);
          for (const [pollId, pollVotes] of voteMap) {
            next.set(pollId, pollVotes);
          }
          return next;
        });
        setLoadError(null);
      } catch {
        setLoadError("Could not load polls. Please refresh.");
      }
    }
    void load();
  }, [authed, user, hydratePollOptions]);

  const handleRealtime = useCallback((event: RealtimeEvent) => {
    const action = normalizeRealtimeAction(event.action);
    if (action == null) {
      return;
    }

    if (event.table === "votes") {
      const vote = event.record as unknown as Vote;
      setVotesMap((prev) => {
        const next = new Map(prev);
        const pollVotes = [...(next.get(vote.poll_id) ?? [])];
        if (action === "create") {
          const existingIndex = pollVotes.findIndex((entry) => entry.user_id === vote.user_id);
          if (existingIndex >= 0) {
            pollVotes[existingIndex] = vote;
          } else {
            pollVotes.push(vote);
          }
        } else if (action === "update") {
          const existingIndex = pollVotes.findIndex((entry) => entry.id === vote.id);
          if (existingIndex >= 0) {
            pollVotes[existingIndex] = vote;
          } else {
            pollVotes.push(vote);
          }
        } else if (action === "delete") {
          next.set(vote.poll_id, pollVotes.filter((entry) => entry.id !== vote.id));
          return next;
        }
        next.set(vote.poll_id, pollVotes);
        return next;
      });
    }

    if (event.table === "polls") {
      const poll = event.record as unknown as Poll;
      if (action === "create") {
        setPolls((prev) => (prev.find((entry) => entry.id === poll.id) ? prev : [poll, ...prev]));
        void hydratePollOptions(poll.id);
      } else if (action === "update") {
        setPolls((prev) => prev.map((entry) => (entry.id === poll.id ? poll : entry)));
      } else if (action === "delete") {
        setPolls((prev) => prev.filter((entry) => entry.id !== poll.id));
      }
    }

    if (event.table === "poll_options") {
      const option = event.record as unknown as PollOption;
      if (action === "create") {
        setOptionsMap((prev) => {
          const next = new Map(prev);
          const pollOptions = [...(next.get(option.poll_id) ?? [])];
          if (!pollOptions.find((entry) => entry.id === option.id)) pollOptions.push(option);
          next.set(option.poll_id, pollOptions);
          return next;
        });
      }
    }
  }, [hydratePollOptions]);

  useRealtime(authed ? ["polls", "poll_options", "votes"] : [], handleRealtime);

  function handlePollCreated(poll: Poll, options: PollOption[]) {
    setPolls((prev) => (prev.find((entry) => entry.id === poll.id) ? prev : [poll, ...prev]));
    setOptionsMap((prev) => {
      const next = new Map(prev);
      next.set(poll.id, options);
      return next;
    });
    setShowCreate(false);
  }

  function handleClosePoll(pollId: string) {
    setPolls((prev) => prev.map((poll) => (poll.id === pollId ? { ...poll, is_closed: true } : poll)));
  }

  function handleVoteCast(vote: Vote) {
    setVotesMap((prev) => {
      const next = new Map(prev);
      const pollVotes = [...(next.get(vote.poll_id) ?? [])];
      const existingIndex = pollVotes.findIndex((entry) => entry.user_id === vote.user_id);
      if (existingIndex >= 0) {
        pollVotes[existingIndex] = vote;
      } else {
        pollVotes.push(vote);
      }
      next.set(vote.poll_id, pollVotes);
      return next;
    });
  }

  async function handleLogout() {
    setLogoutPending(true);
    setLogoutError(null);
    setAnonymousBootstrapEnabled(false);
    disableAnonymousBootstrap();
    try {
      await logout();
      clearPersistedTokens();
      setPolls([]);
      setOptionsMap(new Map());
      setVotesMap(new Map());
    } catch {
      if (anonymousBootstrapEnabled) {
        setAnonymousBootstrapEnabled(true);
        clearAnonymousBootstrapOptOut();
      }
      setLogoutError("Sign out failed. Please try again.");
    } finally {
      setLogoutPending(false);
    }
  }

  function handleAuth(email: string) {
    setAnonymousBootstrapEnabled(true);
    setUserEmail(email);
  }

  if (bootstrapping || loading) {
    return <div className="min-h-screen flex items-center justify-center text-gray-400">Loading...</div>;
  }

  if (!authed) {
    return <AuthForm onAuth={handleAuth} />;
  }

  return (
    <div className="min-h-screen">
        <header className="border-b border-gray-800 px-4 py-3 flex justify-between items-center">
          <h1 className="text-xl font-bold">Live Polls</h1>
          <div className="flex gap-3 items-center">
            {logoutError && (
              <span role="alert" className="text-xs text-red-400">
                {logoutError}
              </span>
            )}
            {loadError && (
              <span role="alert" className="text-xs text-red-400">
                {loadError}
              </span>
            )}
            {userEmail && (
              <span data-testid="user-email" className="text-xs text-gray-500 hidden sm:block">
                {userEmail}
              </span>
            )}
            {canCreatePolls && (
              <button
                onClick={() => setShowCreate(!showCreate)}
                className="bg-blue-600 hover:bg-blue-500 rounded px-3 py-1.5 text-sm font-semibold"
              >
                {showCreate ? "Cancel" : "+ New Poll"}
              </button>
            )}
            <button
              onClick={() => void handleLogout()}
              disabled={logoutPending}
              className="text-gray-400 hover:text-white text-sm disabled:opacity-60"
            >
              {logoutPending ? "Signing out..." : "Sign out"}
            </button>
          </div>
        </header>

        <main className="max-w-2xl mx-auto p-4 flex flex-col gap-4">
          {showCreate && canCreatePolls && userId && <CreatePoll userId={userId} onCreated={handlePollCreated} />}

          {polls.length === 0 && !showCreate && (
            <div className="text-center text-gray-500 py-12">
              <p className="text-lg mb-2">No polls yet</p>
              <p className="text-sm">Create the first one!</p>
            </div>
          )}

          {polls.map((poll) => (
            <PollCard
              key={poll.id}
              poll={poll}
              options={optionsMap.get(poll.id) ?? []}
              votes={votesMap.get(poll.id) ?? []}
              currentUserId={userId}
              onClose={handleClosePoll}
              onVote={handleVoteCast}
            />
          ))}
        </main>
      </div>
  );
}
