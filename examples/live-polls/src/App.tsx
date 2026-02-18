import { useCallback, useEffect, useState } from "react";
import AuthForm from "./components/AuthForm";
import CreatePoll from "./components/CreatePoll";
import PollCard from "./components/PollCard";
import { useRealtime, type RealtimeEvent } from "./hooks/useRealtime";
import { ayb, isLoggedIn, clearPersistedTokens } from "./lib/ayb";
import type { Poll, PollOption, Vote } from "./types";

export default function App() {
  const [authed, setAuthed] = useState(isLoggedIn());
  const [userId, setUserId] = useState<string | null>(null);
  const [polls, setPolls] = useState<Poll[]>([]);
  const [optionsMap, setOptionsMap] = useState<Map<string, PollOption[]>>(new Map());
  const [votesMap, setVotesMap] = useState<Map<string, Vote[]>>(new Map());
  const [showCreate, setShowCreate] = useState(false);

  // Load current user on mount.
  useEffect(() => {
    if (authed) {
      ayb.auth.me().then((u) => setUserId(u.id)).catch(() => {});
    }
  }, [authed]);

  // Load all polls, options, and votes.
  useEffect(() => {
    if (!authed) return;
    async function load() {
      try {
        const [pollRes, optRes, voteRes] = await Promise.all([
          ayb.records.list<Poll>("polls", { sort: "-created_at", perPage: 100 }),
          ayb.records.list<PollOption>("poll_options", { perPage: 5000, skipTotal: true }),
          ayb.records.list<Vote>("votes", { perPage: 5000, skipTotal: true }),
        ]);
        setPolls(pollRes.items);

        const om = new Map<string, PollOption[]>();
        for (const o of optRes.items) {
          const arr = om.get(o.poll_id) ?? [];
          arr.push(o);
          om.set(o.poll_id, arr);
        }
        setOptionsMap(om);

        const vm = new Map<string, Vote[]>();
        for (const v of voteRes.items) {
          const arr = vm.get(v.poll_id) ?? [];
          arr.push(v);
          vm.set(v.poll_id, arr);
        }
        setVotesMap(vm);
      } catch {
        // Polls load empty if server unavailable.
      }
    }
    load();
  }, [authed]);

  // Realtime: update votes, polls, options on changes.
  const handleRealtime = useCallback((event: RealtimeEvent) => {
    if (event.table === "votes") {
      const vote = event.record as unknown as Vote;
      setVotesMap((prev) => {
        const next = new Map(prev);
        const arr = [...(next.get(vote.poll_id) ?? [])];
        if (event.action === "create") {
          // Replace if same user changed their vote, otherwise add.
          const idx = arr.findIndex((v) => v.user_id === vote.user_id);
          if (idx >= 0) {
            arr[idx] = vote;
          } else {
            arr.push(vote);
          }
        } else if (event.action === "update") {
          const idx = arr.findIndex((v) => v.id === vote.id);
          if (idx >= 0) arr[idx] = vote;
        } else if (event.action === "delete") {
          const filtered = arr.filter((v) => v.id !== vote.id);
          next.set(vote.poll_id, filtered);
          return next;
        }
        next.set(vote.poll_id, arr);
        return next;
      });
    }

    if (event.table === "polls") {
      const poll = event.record as unknown as Poll;
      if (event.action === "create") {
        setPolls((prev) => (prev.find((p) => p.id === poll.id) ? prev : [poll, ...prev]));
      } else if (event.action === "update") {
        setPolls((prev) => prev.map((p) => (p.id === poll.id ? poll : p)));
      } else if (event.action === "delete") {
        setPolls((prev) => prev.filter((p) => p.id !== poll.id));
      }
    }

    if (event.table === "poll_options") {
      const opt = event.record as unknown as PollOption;
      if (event.action === "create") {
        setOptionsMap((prev) => {
          const next = new Map(prev);
          const arr = [...(next.get(opt.poll_id) ?? [])];
          if (!arr.find((o) => o.id === opt.id)) arr.push(opt);
          next.set(opt.poll_id, arr);
          return next;
        });
      }
    }
  }, []);

  useRealtime(["polls", "poll_options", "votes"], handleRealtime);

  function handlePollCreated(poll: Poll, options: PollOption[]) {
    setPolls((prev) => [poll, ...prev]);
    setOptionsMap((prev) => {
      const next = new Map(prev);
      next.set(poll.id, options);
      return next;
    });
    setShowCreate(false);
  }

  function handleClosePoll(pollId: string) {
    setPolls((prev) => prev.map((p) => (p.id === pollId ? { ...p, is_closed: true } : p)));
  }

  function handleLogout() {
    clearPersistedTokens();
    setAuthed(false);
    setUserId(null);
    setPolls([]);
  }

  if (!authed) {
    return <AuthForm onAuth={() => setAuthed(true)} />;
  }

  return (
    <div className="min-h-screen">
      <header className="border-b border-gray-800 px-4 py-3 flex justify-between items-center">
        <h1 className="text-xl font-bold">Live Polls</h1>
        <div className="flex gap-3 items-center">
          <button
            onClick={() => setShowCreate(!showCreate)}
            className="bg-blue-600 hover:bg-blue-500 rounded px-3 py-1.5 text-sm font-semibold"
          >
            {showCreate ? "Cancel" : "+ New Poll"}
          </button>
          <button
            onClick={handleLogout}
            className="text-gray-400 hover:text-white text-sm"
          >
            Sign out
          </button>
        </div>
      </header>

      <main className="max-w-2xl mx-auto p-4 flex flex-col gap-4">
        {showCreate && <CreatePoll onCreated={handlePollCreated} />}

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
          />
        ))}
      </main>
    </div>
  );
}
