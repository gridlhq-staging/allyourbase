import { useState } from "react";
import type { ChatMessage } from "../types";

interface Props {
  history: ChatMessage[];
  onHistoryChange: (messages: ChatMessage[]) => void;
  onSend: (messages: ChatMessage[]) => Promise<void>;
  streamedText: string;
  streaming: boolean;
}

export default function ChatPanel({
  history,
  onHistoryChange,
  onSend,
  streamedText,
  streaming,
}: Props) {
  const [input, setInput] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!input.trim() || streaming) return;
    const userMsg: ChatMessage = { role: "user", content: input.trim() };
    const messages = [...history, userMsg];
    onHistoryChange(messages);
    setInput("");
    await onSend(messages);
  }

  return (
    <div className="space-y-3">
      <div className="bg-gray-800 rounded-lg p-3 min-h-[120px] max-h-[300px] overflow-y-auto text-sm">
        {history.length === 0 && !streaming && (
          <p className="text-gray-500">Ask a question about movies...</p>
        )}
        {history.map((msg, i) => (
          <div key={i} className={`mb-2 ${msg.role === "user" ? "text-purple-300" : "text-gray-300"}`}>
            <span className="font-semibold text-xs uppercase text-gray-500 mr-1">{msg.role}:</span>
            {msg.content}
          </div>
        ))}
        {streaming && (
          <div className="text-gray-300">
            <span className="font-semibold text-xs uppercase text-gray-500 mr-1">assistant:</span>
            {streamedText}
            <span className="animate-pulse">|</span>
          </div>
        )}
      </div>
      <form onSubmit={handleSubmit} className="flex gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Ask about movies..."
          disabled={streaming}
          className="flex-1 px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 text-sm focus:outline-none focus:border-purple-500"
        />
        <button
          type="submit"
          disabled={streaming || !input.trim()}
          className="px-4 py-2 bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg text-sm text-white transition-colors"
        >
          Send
        </button>
      </form>
    </div>
  );
}
