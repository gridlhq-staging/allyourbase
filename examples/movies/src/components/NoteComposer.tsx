import { useState } from "react";

interface Props {
  movieSlug: string;
  onSubmit: (text: string, movieSlug: string) => Promise<void>;
}

export default function NoteComposer({ movieSlug, onSubmit }: Props) {
  const [text, setText] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!text.trim()) return;
    setSubmitting(true);
    setError(null);
    setSuccess(false);
    try {
      await onSubmit(text.trim(), movieSlug);
      setText("");
      setSuccess(true);
      setTimeout(() => setSuccess(false), 2000);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to save note");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-2">
      <input
        type="text"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Add a note about this movie..."
        disabled={submitting}
        className="w-full px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 text-sm focus:outline-none focus:border-purple-500"
      />
      <div className="flex items-center gap-2">
        <button
          type="submit"
          disabled={submitting || !text.trim()}
          className="px-3 py-1.5 bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg text-sm text-white transition-colors"
        >
          {submitting ? "Saving..." : "Save Note"}
        </button>
        {error && <span role="alert" className="text-sm text-red-400">{error}</span>}
        {success && <span className="text-sm text-green-400">Saved</span>}
      </div>
    </form>
  );
}
