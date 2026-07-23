import { useEffect, useState } from "react";
import { ayb } from "../lib/ayb";
import type { Board } from "../types";

interface Props {
  onSelectBoard: (board: Board) => void;
}

export default function BoardList({ onSelectBoard }: Props) {
  const [boards, setBoards] = useState<Board[]>([]);
  const [loading, setLoading] = useState(true);
  const [newTitle, setNewTitle] = useState("");
  const [creating, setCreating] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadBoards();
  }, []);

  async function loadBoards() {
    setError(null);
    try {
      const res = await ayb.records.list<Board>("boards", {
        sort: "-created_at",
      });
      setBoards((prev) => {
        // Preserve boards created locally if the initial load resolves after
        // a create request and returns a stale snapshot.
        const merged = new Map(prev.map((board) => [board.id, board]));
        for (const board of res.items) {
          merged.set(board.id, board);
        }
        return Array.from(merged.values()).sort(
          (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
        );
      });
    } catch (err) {
      console.error("Failed to load boards:", err);
      setError(err instanceof Error ? err.message : "Failed to load boards");
    } finally {
      setLoading(false);
    }
  }

  async function createBoard(e: React.FormEvent) {
    e.preventDefault();
    if (!newTitle.trim()) return;
    setCreating(true);
    setError(null);
    try {
      const me = await ayb.auth.me();
      const board = await ayb.records.create<Board>("boards", {
        title: newTitle.trim(),
        user_id: me.id,
      });
      setBoards((prev) => [board, ...prev.filter((existing) => existing.id !== board.id)]);
      setNewTitle("");
    } catch (err) {
      console.error("Failed to create board:", err);
      setError(err instanceof Error ? err.message : "Failed to create board");
    } finally {
      setCreating(false);
    }
  }

  async function deleteBoard(board: Board) {
    if (!confirm(`Delete "${board.title}"?`)) return;
    setError(null);
    try {
      await ayb.records.delete("boards", board.id);
      setBoards((prev) => prev.filter((existing) => existing.id !== board.id));
    } catch (err) {
      console.error("Failed to delete board:", err);
      setError(err instanceof Error ? err.message : "Failed to delete board");
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-64">
        <p className="text-gray-500">Loading boards...</p>
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto py-8 px-4">
      <h2 className="text-xl font-bold text-gray-900 mb-6">Your Boards</h2>

      {error && (
        <p role="alert" className="mb-4 text-sm text-red-700">
          {error}
        </p>
      )}

      <form onSubmit={createBoard} className="flex gap-2 mb-6">
        <input
          type="text"
          value={newTitle}
          onChange={(e) => setNewTitle(e.target.value)}
          placeholder="New board name..."
          className="flex-1 px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500"
        />
        <button
          type="submit"
          disabled={creating || !newTitle.trim()}
          className="bg-blue-600 text-white px-4 py-2 rounded-lg font-medium hover:bg-blue-700 disabled:opacity-50 transition-colors"
        >
          {creating ? "..." : "Create"}
        </button>
      </form>

      {boards.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-xl border-2 border-dashed border-gray-200">
          <p className="text-gray-500 text-lg">No boards yet</p>
          <p className="text-gray-500 text-sm mt-1">
            Create your first board above
          </p>
        </div>
      ) : (
        <div className="grid gap-3">
          {boards.map((board) => (
            <div
              key={board.id}
              className="flex items-center justify-between bg-white rounded-xl shadow-sm hover:shadow-md focus-within:shadow-md transition-shadow group"
            >
              <button
                type="button"
                data-testid={`board-${board.id}`}
                onClick={() => onSelectBoard(board)}
                aria-label={`Open board ${board.title}`}
                className="flex-1 p-4 text-left rounded-l-xl focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-inset"
              >
                <span className="block font-semibold text-gray-900">{board.title}</span>
                <span className="block text-xs text-gray-500 mt-1">
                  {new Date(board.created_at).toLocaleDateString()}
                </span>
              </button>
              <button
                type="button"
                onClick={() => void deleteBoard(board)}
                aria-label={`Delete board ${board.title}`}
                className="mr-4 text-gray-500 hover:text-red-700 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-all p-1 rounded focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-red-500"
                title="Delete board"
              >
                <svg
                  className="w-5 h-5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    strokeWidth={2}
                    d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                  />
                </svg>
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
