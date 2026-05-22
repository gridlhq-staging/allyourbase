import { useCallback, useEffect, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import AuthForm from "./components/AuthForm";
import CreatePoll from "./components/CreatePoll";
import PollCard from "./components/PollCard";
import { useRealtime, type RealtimeEvent } from "./hooks/useRealtime";
import {
  ayb,
  clearAnonymousBootstrapOptOut,
  clearPersistedTokens,
  disableAnonymousBootstrap,
  isAnonymousBootstrapEnabled,
} from "./lib/ayb";
import type { Poll, PollOption, Vote } from "./types";

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

export default function App() {
  const { user, token, loading, logout } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  const { bootstrapping } = useAybAnonymousBootstrap({ enabled: anonymousBootstrapEnabled });
  const authed = Boolean(token) && Boolean(user) && !user.isAnonymous;
  const [userId, setUserId] = useState<string | null>(null);
  const [userEmail, setUserEmail] = useState<string | null>(null);
  const [polls, setPolls] = useState<Poll[]>([]);
  const [optionsMap, setOptionsMap] = useState<Map<string, PollOption[]>>(new Map());
  const [votesMap, setVotesMap] = useState<Map<string, Vote[]>>(new Map());
  const [showCreate, setShowCreate] = useState(false);
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

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
        const [pollRes, allOpts, allVotes] = await Promise.all([
          ayb.records.list<Poll>("polls", { sort: "-created_at", perPage: 100 }),
          fetchAll<PollOption>("poll_options"),
          fetchAll<Vote>("votes"),
        ]);
        setPolls(pollRes.items);

        const optionMap = new Map<string, PollOption[]>();
        for (const option of allOpts) {
          const pollOptions = optionMap.get(option.poll_id) ?? [];
          pollOptions.push(option);
          optionMap.set(option.poll_id, pollOptions);
        }
        setOptionsMap((prev) => {
          const next = new Map(prev);
          for (const [pollId, pollOptions] of optionMap) {
            next.set(pollId, pollOptions);
          }
          return next;
        });

        const voteMap = new Map<string, Vote[]>();
        for (const vote of allVotes) {
          const pollVotes = voteMap.get(vote.poll_id) ?? [];
          pollVotes.push(vote);
          voteMap.set(vote.poll_id, pollVotes);
        }
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
  }, [authed]);

  const handleRealtime = useCallback((event: RealtimeEvent) => {
    if (event.table === "votes") {
      const vote = event.record as unknown as Vote;
      setVotesMap((prev) => {
        const next = new Map(prev);
        const pollVotes = [...(next.get(vote.poll_id) ?? [])];
        if (event.action === "create") {
          const existingIndex = pollVotes.findIndex((entry) => entry.user_id === vote.user_id);
          if (existingIndex >= 0) {
            pollVotes[existingIndex] = vote;
          } else {
            pollVotes.push(vote);
          }
        } else if (event.action === "update") {
          const existingIndex = pollVotes.findIndex((entry) => entry.id === vote.id);
          if (existingIndex >= 0) {
            pollVotes[existingIndex] = vote;
          } else {
            pollVotes.push(vote);
          }
        } else if (event.action === "delete") {
          next.set(vote.poll_id, pollVotes.filter((entry) => entry.id !== vote.id));
          return next;
        }
        next.set(vote.poll_id, pollVotes);
        return next;
      });
    }

    if (event.table === "polls") {
      const poll = event.record as unknown as Poll;
      if (event.action === "create") {
        setPolls((prev) => (prev.find((entry) => entry.id === poll.id) ? prev : [poll, ...prev]));
      } else if (event.action === "update") {
        setPolls((prev) => prev.map((entry) => (entry.id === poll.id ? poll : entry)));
      } else if (event.action === "delete") {
        setPolls((prev) => prev.filter((entry) => entry.id !== poll.id));
      }
    }

    if (event.table === "poll_options") {
      const option = event.record as unknown as PollOption;
      if (event.action === "create") {
        setOptionsMap((prev) => {
          const next = new Map(prev);
          const pollOptions = [...(next.get(option.poll_id) ?? [])];
          if (!pollOptions.find((entry) => entry.id === option.id)) pollOptions.push(option);
          next.set(option.poll_id, pollOptions);
          return next;
        });
      }
    }
  }, []);

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
    const bootstrapEnabledBeforeLogout = anonymousBootstrapEnabled;
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
      if (bootstrapEnabledBeforeLogout) {
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
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="bg-blue-600 hover:bg-blue-500 rounded px-3 py-1.5 text-sm font-semibold"
          >
            {showCreate ? "Cancel" : "+ New Poll"}
          </button>
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
        {showCreate && userId && <CreatePoll userId={userId} onCreated={handlePollCreated} />}

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
