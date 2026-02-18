import { isLoggedIn, clearPersistedTokens } from "../lib/ayb";

interface Props {
  pixelCount: number;
  user: string | null;
  onLoginClick: () => void;
  onLogout: () => void;
}

export default function Header({ pixelCount, user, onLoginClick, onLogout }: Props) {
  return (
    <header className="fixed top-0 left-0 right-0 z-30 flex items-center justify-between px-4 py-2 bg-gray-900/90 backdrop-blur-sm border-b border-gray-800">
      <div className="flex items-center gap-3">
        <h1 className="text-sm font-bold tracking-wide">
          <span className="text-blue-400">place</span>
          <span className="text-gray-500">.allyourbase.io</span>
        </h1>
        <span className="text-xs text-gray-500">{pixelCount.toLocaleString()} pixels placed</span>
      </div>
      <div className="flex items-center gap-3">
        {isLoggedIn() && user ? (
          <>
            <span className="text-xs text-gray-400">{user}</span>
            <button
              onClick={() => {
                clearPersistedTokens();
                onLogout();
              }}
              className="text-xs text-gray-500 hover:text-white"
            >
              Logout
            </button>
          </>
        ) : (
          <button
            onClick={onLoginClick}
            className="text-xs bg-blue-600 hover:bg-blue-500 px-3 py-1 rounded font-semibold"
          >
            Sign in to place
          </button>
        )}
      </div>
    </header>
  );
}
