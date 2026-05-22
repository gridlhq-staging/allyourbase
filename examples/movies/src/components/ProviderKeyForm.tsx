import { useState } from "react";
import type { BYOKProvider } from "../types";

interface Props {
  onSet: (provider: BYOKProvider, secretName: string) => Promise<void>;
  onClear: (provider: BYOKProvider) => Promise<void>;
}

const providers: BYOKProvider[] = ["openai", "anthropic", "ollama"];

export default function ProviderKeyForm({ onSet, onClear }: Props) {
  const [provider, setProvider] = useState<BYOKProvider>("openai");
  const [secretName, setSecretName] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSet(e: React.FormEvent) {
    e.preventDefault();
    if (!secretName.trim()) return;
    setSubmitting(true);
    setError(null);
    try {
      await onSet(provider, secretName.trim());
      setSecretName("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to set key");
    } finally {
      setSubmitting(false);
    }
  }

  async function handleClear() {
    setSubmitting(true);
    setError(null);
    try {
      await onClear(provider);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to clear key");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex gap-2 items-center">
        <select
          value={provider}
          onChange={(e) => setProvider(e.target.value as BYOKProvider)}
          className="px-2 py-1.5 bg-gray-800 border border-gray-700 rounded-lg text-white text-sm focus:outline-none focus:border-purple-500"
        >
          {providers.map((p) => (
            <option key={p} value={p}>{p}</option>
          ))}
        </select>
      </div>
      <form onSubmit={handleSet} className="flex gap-2">
        <input
          type="text"
          value={secretName}
          onChange={(e) => setSecretName(e.target.value)}
          placeholder="Vault secret name..."
          disabled={submitting}
          className="flex-1 px-3 py-2 bg-gray-800 border border-gray-700 rounded-lg text-white placeholder-gray-500 text-sm focus:outline-none focus:border-purple-500"
        />
        <button
          type="submit"
          disabled={submitting || !secretName.trim()}
          className="px-3 py-1.5 bg-purple-600 hover:bg-purple-700 disabled:opacity-50 rounded-lg text-sm text-white transition-colors"
        >
          Set
        </button>
        <button
          type="button"
          onClick={handleClear}
          disabled={submitting}
          className="px-3 py-1.5 bg-gray-700 hover:bg-gray-600 disabled:opacity-50 rounded-lg text-sm text-white transition-colors"
        >
          Clear
        </button>
      </form>
      {error && <span role="alert" className="text-sm text-red-400">{error}</span>}
    </div>
  );
}
