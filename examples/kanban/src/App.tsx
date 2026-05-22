import { useEffect, useState } from "react";
import { useAuth, useAybAnonymousBootstrap } from "@allyourbase/react";
import {
  clearAnonymousBootstrapOptOut,
  clearPersistedTokens,
  disableAnonymousBootstrap,
  getPersistedEmail,
  isAnonymousBootstrapEnabled,
} from "./lib/ayb";
import type { Board } from "./types";
import AuthForm from "./components/AuthForm";
import BoardList from "./components/BoardList";
import BoardView from "./components/BoardView";

export default function App() {
  const { user, token, loading, logout } = useAuth();
  const [anonymousBootstrapEnabled, setAnonymousBootstrapEnabled] = useState(isAnonymousBootstrapEnabled);
  const { bootstrapping } = useAybAnonymousBootstrap({ enabled: anonymousBootstrapEnabled });
  const [email, setEmail] = useState<string | null>(getPersistedEmail());
  const [selectedBoard, setSelectedBoard] = useState<Board | null>(null);
  const [logoutPending, setLogoutPending] = useState(false);
  const [logoutError, setLogoutError] = useState<string | null>(null);

  useEffect(() => {
    if (user?.email) {
      setEmail(user.email);
      return;
    }
    if (!token) {
      setEmail(getPersistedEmail());
    }
  }, [user, token]);

  async function handleLogout() {
    const bootstrapEnabledBeforeLogout = anonymousBootstrapEnabled;
    setLogoutPending(true);
    setLogoutError(null);
    setAnonymousBootstrapEnabled(false);
    disableAnonymousBootstrap();
    try {
      await logout();
      clearPersistedTokens();
      setEmail(null);
      setSelectedBoard(null);
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

  function handleAuth(emailValue: string) {
    setAnonymousBootstrapEnabled(true);
    setEmail(emailValue);
  }

  if (bootstrapping || loading) {
    return <div className="min-h-screen flex items-center justify-center text-gray-500">Loading...</div>;
  }

  if (!token || !user || user.isAnonymous) {
    return <AuthForm onAuth={handleAuth} />;
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
