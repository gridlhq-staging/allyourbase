import { useState } from "react";
import { AybLoginBar, DemoSuggestionChip, useAuth } from "@allyourbase/react";
import { clearAnonymousBootstrapOptOut, persistTokens } from "../lib/ayb";

interface Props {
  onAuth: (email: string) => void;
}

const demoAccounts = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
  { email: "charlie@demo.test", password: "password123" },
];

function CopyIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function CheckIcon({ className }: { className?: string }) {
  return (
    <svg className={className} width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
      <polyline points="20 6 9 17 4 12" />
    </svg>
  );
}

function CopyButton({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  function handleCopy(e: React.MouseEvent) {
    e.stopPropagation();
    navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <span
      role="button"
      tabIndex={0}
      onClick={handleCopy}
      onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") handleCopy(e as unknown as React.MouseEvent); }}
      className="p-0.5 rounded hover:bg-gray-200 text-gray-400 hover:text-gray-600 transition-colors cursor-pointer inline-flex"
      title={copied ? "Copied!" : "Copy"}
    >
      {copied ? <CheckIcon className="text-green-600" /> : <CopyIcon />}
    </span>
  );
}

const OAUTH_PROVIDERS: ("github" | "google")[] = ["github", "google"];

export default function AuthForm({ onAuth }: Props) {
  const { user, login, register, loading, signInWithOAuth, signInAnonymously, requestMagicLink, linkEmail } = useAuth();
  const isAnonymous = Boolean(user?.isAnonymous);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function handleSubmit() {
    setError("");
    setNotice("");
    try {
      if (mode === "register") {
        await register(email, password);
      } else {
        await login(email, password);
      }
      persistTokens(email);
      clearAnonymousBootstrapOptOut();
      onAuth(email);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Authentication failed");
    }
  }

  async function handleOAuthProvider(provider: "github" | "google") {
    setError("");
    setNotice("");
    try {
      await signInWithOAuth(provider);
    } catch (err) {
      setError(err instanceof Error ? err.message : "OAuth sign-in failed");
    }
  }

  async function handleAnonymous() {
    setError("");
    setNotice("");
    try {
      await signInAnonymously();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Guest sign-in failed");
    }
  }

  async function handleRequestMagicLink(value: string) {
    setError("");
    setNotice("");
    try {
      await requestMagicLink(value);
      setNotice(`We sent a magic link to ${value}. Check your inbox.`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Magic link request failed");
    }
  }

  async function handleUpgradeAnonymous() {
    setError("");
    setNotice("");
    try {
      await linkEmail(email, password);
      persistTokens(email);
      clearAnonymousBootstrapOptOut();
      onAuth(email);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Account upgrade failed");
    }
  }

  function fillAccount(acct: { email: string; password: string }) {
    setEmail(acct.email);
    setPassword(acct.password);
    setError("");
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-blue-50 to-indigo-100">
      <div className="bg-white rounded-xl shadow-lg p-8 w-full max-w-md">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-gray-900">Kanban Board</h1>
          <p className="text-sm text-gray-500 mt-1">
            Powered by <span className="font-semibold">Allyourbase</span>
          </p>
        </div>

        <AybLoginBar
          methods={{
            password: true,
            oauth: true,
            anonymous: !isAnonymous,
            canUpgradeAnonymous: isAnonymous,
            magicLink: true,
          }}
          loading={loading}
          mode={mode}
          email={email}
          password={password}
          error={error || null}
          demoSuggestions={[]}
          oauthProviders={OAUTH_PROVIDERS}
          onEmailChange={setEmail}
          onPasswordChange={setPassword}
          onModeChange={(nextMode) => {
            setMode(nextMode);
            setError("");
            setNotice("");
          }}
          onSubmit={handleSubmit}
          onOAuth={async () => {}}
          onAnonymous={handleAnonymous}
          onOAuthProvider={handleOAuthProvider}
          onRequestMagicLink={handleRequestMagicLink}
          onUpgradeAnonymous={handleUpgradeAnonymous}
        />
        {notice && <p className="text-xs text-emerald-600 mt-3" role="status">{notice}</p>}

        {mode === "login" && (
          <div className="mt-5 border-t border-gray-200 pt-4">
            <p className="text-[11px] uppercase tracking-wider text-gray-400 font-semibold mb-2">
              Demo accounts
            </p>
            <div className="flex flex-col gap-1">
              {demoAccounts.map((acct) => (
                <div key={acct.email} className="w-full text-left px-2.5 py-2 rounded-lg bg-gray-50 hover:bg-gray-100 border border-transparent hover:border-gray-200 transition-colors group flex items-center gap-2">
                  <div className="flex-1 min-w-0">
                    <DemoSuggestionChip
                      suggestion={{ label: acct.email, email: acct.email, password: acct.password }}
                      onSelect={fillAccount}
                    />
                    <span className="text-[11px] font-mono text-gray-400">{acct.password}</span>
                  </div>
                  <CopyButton value={`${acct.email}\t${acct.password}`} />
                </div>
              ))}
            </div>
            <p className="text-[10px] text-gray-400 mt-2 text-center">
              Click to fill, then sign in
            </p>
          </div>
        )}

        <div className="mt-4 bg-blue-50 border border-blue-100 rounded-lg px-4 py-3">
          <p className="text-xs font-semibold text-blue-900 mb-1.5">Try it out</p>
          <ul className="text-[11px] text-blue-800 space-y-1 list-disc list-inside">
            {mode === "register" ? (
              <li>Create an account, then build your first board</li>
            ) : (
              <li>Sign in with a demo account above</li>
            )}
            <li>Create a board and add some cards</li>
            <li>Open a second browser and sign in as a different user</li>
            <li>Edit cards in one window - watch them update instantly in the other</li>
          </ul>
        </div>
      </div>
    </div>
  );
}
