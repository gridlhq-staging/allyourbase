import { useEffect, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import {
  clearPersistedTokens,
  disableAnonymousBootstrap,
  getPersistedEmail,
  isAnonymousBootstrapEnabled,
} from "./lib/ayb";
import { ensureSampleBoard } from "./lib/seed";
import type { Board } from "./types";
import AuthForm from "./components/AuthForm";
import BoardList from "./components/BoardList";
import BoardView from "./components/BoardView";

const MAX_SEED_ATTEMPTS = 3;
const SEED_RETRY_DELAY_MS = 250;

export default function App() {
  const { user, token, loading, logout, signInAnonymously } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  const { bootstrapping } = useAybAnonymousBootstrap({
    enabled: anonymousBootstrapEnabled,
    token,
    signInAnonymously,
  });
  const [email, setEmail] = useState<string | null>(getPersistedEmail());
  const [selectedBoard, setSelectedBoard] = useState<Board | null>(null);
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);
  // Gates the board list until the idempotent seed check has run once for the
  // current session, so the list never flashes empty-then-populated.
  const [seedChecked, setSeedChecked] = useState(false);
  const [seedAttempt, setSeedAttempt] = useState(0);

  useEffect(() => {
    if (user?.email) {
      setEmail(user.email);
      return;
    }
    if (!token) {
      setEmail(getPersistedEmail());
    }
  }, [user, token]);

  // Seed a starter board for a fresh user once auth has fully resolved.
  // ensureSampleBoard() is idempotent — it no-ops when the user owns a board.
  useEffect(() => {
    if (bootstrapping || loading) return;
    if (!token || !user) return;
    if (seedChecked) return;
    let cancelled = false;
    let retryTimer: number | undefined;
    ensureSampleBoard()
      .then(() => {
        if (!cancelled) setSeedChecked(true);
      })
      .catch((err) => {
        // A demo seed failure must not strand the user — fall through to the
        // board list after bounded retries rather than blocking the shell.
        console.error("Sample board seeding failed:", err);
        if (cancelled) return;
        if (seedAttempt + 1 < MAX_SEED_ATTEMPTS) {
          retryTimer = window.setTimeout(() => {
            setSeedAttempt((attempt) => attempt + 1);
          }, SEED_RETRY_DELAY_MS);
          return;
        }
        setSeedChecked(true);
      });
    return () => {
      cancelled = true;
      if (retryTimer !== undefined) {
        window.clearTimeout(retryTimer);
      }
    };
  }, [bootstrapping, loading, token, user, seedChecked, seedAttempt]);

  async function handleLogout() {
    setLogoutPending(true);
    setLogoutError(null);
    setAnonymousBootstrapEnabled(false);
    disableAnonymousBootstrap();
    try {
      await logout();
    } catch {
      setLogoutError("Sign out failed. Please try again.");
    } finally {
      clearPersistedTokens();
      setEmail(null);
      setSelectedBoard(null);
      // Re-arm the seed check so a later sign-in re-runs the idempotent
      // check; a signed-out user is not re-seeded until they sign back in.
      setSeedChecked(false);
      setSeedAttempt(0);
      setLogoutPending(false);
    }
  }

  function handleAuth(emailValue: string) {
    setAnonymousBootstrapEnabled(true);
    setEmail(emailValue);
  }

  if (bootstrapping || loading) {
    return <div className="min-h-screen flex items-center justify-center text-gray-500">Loading...</div>;
  }

  if (!token || !user) {
    return <AuthForm onAuth={handleAuth} />;
  }

  if (!seedChecked) {
    return <div className="min-h-screen flex items-center justify-center text-gray-500">Loading...</div>;
  }

  if (selectedBoard) {
    return (
      <BoardView
        board={selectedBoard}
        onBack={() => setSelectedBoard(null)}
      />
    );
  }

  return (
    <div className="min-h-screen bg-gray-50">
      <header className="bg-white border-b px-6 py-3 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-lg font-bold text-gray-900">Kanban Board</h1>
          <span className="text-xs text-gray-400">
            powered by Allyourbase
          </span>
        </div>
        <div className="flex items-center gap-3">
          {logoutError && (
            <span role="alert" className="text-sm text-red-600">
              {logoutError}
            </span>
          )}
          {email && (
            <span data-testid="user-email" className="text-sm text-gray-500">
              {email}
            </span>
          )}
          <button
            onClick={() => void handleLogout()}
            disabled={logoutPending}
            className="text-sm text-gray-500 hover:text-gray-700 transition-colors"
          >
            {logoutPending ? "Signing out..." : "Sign out"}
          </button>
        </div>
      </header>
      <BoardList onSelectBoard={setSelectedBoard} />
    </div>
  );
}
