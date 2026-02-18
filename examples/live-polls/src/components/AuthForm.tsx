import { FormEvent, useState } from "react";
import { ayb, persistTokens } from "../lib/ayb";

interface Props {
  onAuth: () => void;
}

export default function AuthForm({ onAuth }: Props) {
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      if (mode === "login") {
        await ayb.auth.login(email, password);
      } else {
        await ayb.auth.register(email, password);
      }
      persistTokens();
      onAuth();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "Something went wrong");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <div className="bg-gray-900 border border-gray-700 rounded-xl p-6 w-full max-w-sm shadow-2xl">
        <h1 className="text-2xl font-bold mb-1">Live Polls</h1>
        <p className="text-gray-400 text-sm mb-6">
          {mode === "login" ? "Sign in to create and vote on polls" : "Create your account"}
        </p>
        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <input
            type="email"
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
            required
            autoFocus
          />
          <input
            type="password"
            placeholder="Password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="bg-gray-800 border border-gray-600 rounded px-3 py-2 text-sm focus:outline-none focus:border-blue-500"
            required
            minLength={8}
          />
          {error && <p className="text-red-400 text-xs">{error}</p>}
          <button
            type="submit"
            disabled={loading}
            className="bg-blue-600 hover:bg-blue-500 disabled:opacity-50 rounded py-2 text-sm font-semibold"
          >
            {loading ? "..." : mode === "login" ? "Sign In" : "Create Account"}
          </button>
        </form>
        <div className="mt-3 text-center text-xs text-gray-400">
          {mode === "login" ? (
            <>
              No account?{" "}
              <button onClick={() => setMode("register")} className="text-blue-400 hover:underline">
                Register
              </button>
            </>
          ) : (
            <>
              Have an account?{" "}
              <button onClick={() => setMode("login")} className="text-blue-400 hover:underline">
                Sign in
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
