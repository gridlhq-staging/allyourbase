import { useState } from "react";
import { AybLoginBar, DemoSuggestionChip, useAuth } from "@allyourbase/react";
import { clearAnonymousBootstrapOptOut, persistTokens } from "../lib/ayb";

interface Props {
  onAuth: (email: string) => void;
}

const demoAccounts = [
  { email: "alice@demo.test", password: "password123" },
  { email: "bob@demo.test", password: "password123" },
];

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
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-purple-950 to-gray-950">
      <div className="bg-gray-900 rounded-xl shadow-lg p-8 w-full max-w-md border border-gray-800">
        <div className="text-center mb-6">
          <h1 className="text-2xl font-bold text-white">Movies Demo</h1>
          <p className="text-sm text-gray-400 mt-1">
            Powered by <span className="font-semibold">Allyourbase</span>
          </p>
          {isAnonymous && (
            <p className="text-xs text-purple-300 mt-2">You're browsing as a guest. Add an email and password to keep your data.</p>
          )}
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

        {notice && <p className="text-xs text-emerald-400 mt-3" role="status">{notice}</p>}

        {mode === "login" && !isAnonymous && (
          <div className="mt-5 border-t border-gray-700 pt-4">
            <p className="text-[11px] uppercase tracking-wider text-gray-500 font-semibold mb-2">
              Demo accounts
            </p>
            <div className="flex flex-col gap-1">
              {demoAccounts.map((acct) => (
                <div key={acct.email} className="w-full text-left px-2.5 py-2 rounded-lg bg-gray-800 hover:bg-gray-750 border border-transparent hover:border-gray-700 transition-colors">
                  <DemoSuggestionChip
                    suggestion={{ label: acct.email, email: acct.email, password: acct.password }}
                    onSelect={fillAccount}
                  />
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
