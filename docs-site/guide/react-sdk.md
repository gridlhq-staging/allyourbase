<!-- audited 2026-06-05 -->

# React SDK

The `@allyourbase/react` package provides React primitives on top of `@allyourbase/js`:

- `AYBProvider`
- `useAYBClient`
- `useAuth`
- `useAybAnonymousBootstrap`
- `useQuery`
- `useRealtime`
- `AybLoginBar`
- `DemoSuggestionChip`

## Install

Preview — install from source. Registry publishing is tracked for GA.

```bash
git clone https://github.com/griddlehq/allyourbase.git
cd allyourbase/sdk && npm install && npm run build
cd ../sdk_react && npm install && npm run build

# inside your app
npm install react react-dom /absolute/path/to/allyourbase/sdk /absolute/path/to/allyourbase/sdk_react
```

## Initialize

```tsx
import { AYBClient } from "@allyourbase/js";
import { AYBProvider } from "@allyourbase/react";

const ayb = new AYBClient("http://localhost:8090");

export function App({ children }: { children: React.ReactNode }) {
  return <AYBProvider client={ayb}>{children}</AYBProvider>;
}
```

`useAuth`, `useQuery`, and `useRealtime` must run under `AYBProvider`.

## `useAuth`

`useAuth()` tracks user/session state and wraps auth actions.

```tsx
import { useAuth } from "@allyourbase/react";

export function LoginPanel() {
  const {
    loading,
    user,
    error,
    token,
    refreshToken,
    login,
    register,
    signInAnonymously,
    requestMagicLink,
    confirmMagicLink,
    linkEmail,
    signInWithOAuth,
    logout,
    refresh,
  } = useAuth();

  if (loading) return <p>Loading session...</p>;

  return (
    <div>
      <p>user: {user?.email ?? "anonymous"}</p>
      <p>token: {token ? "set" : "missing"}</p>
      <p>refresh: {refreshToken ? "set" : "missing"}</p>
      {error && <p>{error.message}</p>}
      <button onClick={() => login("user@example.com", "password")}>Login</button>
      <button onClick={() => register("new@example.com", "password")}>Register</button>
      <button onClick={() => refresh()}>Refresh</button>
      <button onClick={() => logout()}>Logout</button>
    </div>
  );
}
```

The destructured shape matches the `UseAuthResult` type re-exported from
`@allyourbase/react`.

### Anonymous-first sign-in

`signInAnonymously` issues a guest session without an email or password. Pair
it with [`useAybAnonymousBootstrap`](#useaybanonymousbootstrap) to auto-create
a guest session on first mount, or call it from a button handler:

```tsx
import { useAuth } from "@allyourbase/react";

export function GuestButton() {
  const { signInAnonymously, loading } = useAuth();
  return (
    <button disabled={loading} onClick={() => signInAnonymously()}>
      Continue as Guest
    </button>
  );
}
```

Once signed in anonymously, call `linkEmail(email, password)` to convert the
guest account to a permanent email/password account, or `requestMagicLink` /
`confirmMagicLink` for passwordless flows. See
[Link email + password](/guide/authentication#link-email-password) and
[Magic link](/guide/authentication#magic-link) for endpoint details.

## `useAybAnonymousBootstrap`

`useAybAnonymousBootstrap({ enabled, token?, signInAnonymously? })` calls
`auth.signInAnonymously()` once on mount when `enabled` is true and there is
no active session token. It returns `{ bootstrapping }`, which is true while
the initial guest sign-in is in flight.

```tsx
import { useAybAnonymousBootstrap } from "@allyourbase/react";

export function AppShell({ children }: { children: React.ReactNode }) {
  const anonymousBootstrapEnabled = true;
  const { bootstrapping } = useAybAnonymousBootstrap({
    enabled: anonymousBootstrapEnabled,
  });

  if (bootstrapping) return <p>Creating guest session...</p>;
  return <>{children}</>;
}
```

`token` and `signInAnonymously` are optional overrides used in tests; in
production both default to the provider client. The reference wiring in
`examples/live-polls/src/App.tsx` gates the hook behind an
`anonymousBootstrapEnabled` flag so users who explicitly sign out can opt out.

## `AybLoginBar`

`AybLoginBar` is a controlled login UI that renders email/password inputs,
OAuth buttons, a guest-sign-in button, and an optional magic-link button based
on the `methods` prop. All state and handlers are owned by the caller.

Required props:

- `methods: AybAuthMethods` — `{ password, oauth, anonymous, canUpgradeAnonymous, magicLink? }`
- `loading: boolean`
- `email: string`, `password: string`, `error: string | null`
- `demoSuggestions: DemoSuggestion[]`
- `onEmailChange`, `onPasswordChange`
- `onSubmit`, `onOAuth`, `onAnonymous`

Optional handlers:

- `onModeChange(mode)` — render the login/register toggle
- `onOAuthProvider(provider)` — render per-provider buttons (used with `oauthProviders`)
- `onRequestMagicLink(email)` — render the "Email me a magic link" button (requires `methods.magicLink`)
- `onUpgradeAnonymous()` — render the upgrade button (requires `methods.canUpgradeAnonymous`)

```tsx
import { useState } from "react";
import { AybLoginBar, useAuth } from "@allyourbase/react";

const OAUTH_PROVIDERS: ("github" | "google")[] = ["github", "google"];

export function LoginForm() {
  const { login, register, signInAnonymously, signInWithOAuth, requestMagicLink, loading } = useAuth();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  return (
    <AybLoginBar
      methods={{ password: true, oauth: true, anonymous: true, canUpgradeAnonymous: false, magicLink: true }}
      loading={loading}
      mode={mode}
      email={email}
      password={password}
      error={error}
      demoSuggestions={[]}
      oauthProviders={OAUTH_PROVIDERS}
      onEmailChange={setEmail}
      onPasswordChange={setPassword}
      onModeChange={(next) => { setMode(next); setError(null); }}
      onSubmit={async () => {
        try {
          mode === "register" ? await register(email, password) : await login(email, password);
        } catch (err) {
          setError(err instanceof Error ? err.message : "Sign-in failed");
        }
      }}
      onOAuth={async () => {}}
      onAnonymous={async () => {
        try { await signInAnonymously(); } catch (err) {
          setError(err instanceof Error ? err.message : "Guest sign-in failed");
        }
      }}
      onOAuthProvider={async (provider) => {
        try { await signInWithOAuth(provider); } catch (err) {
          setError(err instanceof Error ? err.message : "OAuth sign-in failed");
        }
      }}
      onRequestMagicLink={async (value) => {
        try { await requestMagicLink(value); } catch (err) {
          setError(err instanceof Error ? err.message : "Magic link request failed");
        }
      }}
    />
  );
}
```

Cross-links: [Magic link](/guide/authentication#magic-link),
[Link email + password](/guide/authentication#link-email-password).

### `DemoSuggestionChip`

`DemoSuggestionChip` is the standalone version of the chips `AybLoginBar`
renders internally when you pass `demoSuggestions`. Render it directly when you
want chips outside the login bar layout. Props: `{ suggestion: DemoSuggestion,
onSelect(suggestion) }`. `DemoSuggestion` is `{ label: string; email: string;
password: string }`.

```tsx
import { DemoSuggestionChip } from "@allyourbase/react";

<DemoSuggestionChip
  suggestion={{ label: "alice@demo.test", email: "alice@demo.test", password: "password123" }}
  onSelect={(s) => { /* prefill your inputs */ }}
/>
```

## `useQuery`

`useQuery(collection, params?, options?)` wraps `client.records.list()`.

```tsx
import { useQuery } from "@allyourbase/react";

type Post = {
  id: number;
  title: string;
  published: boolean;
};

export function PostList() {
  const { data, loading, error, refetch } = useQuery<Post>(
    "posts",
    { filter: "published=true", sort: "-created_at", perPage: 20 },
    { enabled: true },
  );

  if (loading) return <p>Loading...</p>;
  if (error) return <p>{error.message}</p>;

  return (
    <div>
      <button onClick={() => refetch()}>Refresh</button>
      <ul>
        {data?.items.map((post) => (
          <li key={post.id}>{post.title}</li>
        ))}
      </ul>
    </div>
  );
}
```

### Suspense mode

```tsx
const { data } = useQuery("posts", { sort: "-created_at" }, { suspense: true });
```

When `suspense: true`, the hook throws the fetch promise/errors for a Suspense boundary.

## Mutations

`@allyourbase/react` currently ships `useQuery` (read path) and `useAYBClient` (raw client access).

```tsx
import { useState } from "react";
import { AYBClient } from "@allyourbase/js";
import { useAYBClient, useQuery } from "@allyourbase/react";

export function TodoMutations() {
  const client = useAYBClient() as AYBClient;
  const { data, refetch } = useQuery<{ id: string; title: string; done: boolean }>("todos");
  const [title, setTitle] = useState("");

  const createTodo = async () => {
    await client.records.create("todos", { title, done: false });
    setTitle("");
    await refetch();
  };

  const toggleTodo = async (id: string, done: boolean) => {
    await client.records.update("todos", id, { done: !done });
    await refetch();
  };

  const deleteTodo = async (id: string) => {
    await client.records.delete("todos", id);
    await refetch();
  };

  return (
    <div>
      <input value={title} onChange={(e) => setTitle(e.target.value)} />
      <button onClick={createTodo}>Add</button>
      <ul>
        {data?.items.map((todo) => (
          <li key={todo.id}>
            <button onClick={() => toggleTodo(todo.id, todo.done)}>
              {todo.done ? "Undo" : "Done"}
            </button>
            <button onClick={() => deleteTodo(todo.id)}>Delete</button>
            {todo.title}
          </li>
        ))}
      </ul>
    </div>
  );
}
```

## Error handling

The JS SDK throws `AYBError` for non-2xx responses. Use its fields (`status`, `code`, `data`, `docUrl`) for user-safe handling.

```tsx
import { AYBClient, AYBError } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function SaveButton() {
  const client = useAYBClient() as AYBClient;

  const onSave = async () => {
    try {
      await client.records.create("posts", { title: "Hello" });
    } catch (err) {
      if (err instanceof AYBError) {
        if (err.status === 429) {
          alert("Rate limited. Try again shortly.");
          return;
        }

        if (err.code === "validation/failed") {
          console.error("Validation details", err.data);
        }

        console.error("AYB error", err.status, err.code, err.docUrl);
        return;
      }

      console.error("Unexpected error", err);
    }
  };

  return <button onClick={onSave}>Save</button>;
}
```

## OAuth sign-in

OAuth popup/redirect behavior is implemented in `@allyourbase/js` (`signInWithOAuth`).

```tsx
import { useState } from "react";
import { AYBClient, AYBError } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function OAuthButtons() {
  const client = useAYBClient() as AYBClient;
  const [error, setError] = useState<string | null>(null);

  const signInGoogle = async () => {
    setError(null);
    try {
      await client.auth.signInWithOAuth("google");
    } catch (err) {
      if (err instanceof AYBError) {
        setError(`${err.code ?? "oauth/error"}: ${err.message}`);
        return;
      }
      setError("OAuth sign-in failed");
    }
  };

  return (
    <div>
      <button onClick={signInGoogle}>Continue with Google</button>
      {error && <p>{error}</p>}
    </div>
  );
}
```

If you cannot use popups (for example, native wrappers), pass a `urlCallback` and handle redirect manually:

```ts
await client.auth.signInWithOAuth("github", {
  urlCallback: async (url) => {
    window.location.assign(url);
  },
});
```

## `useRealtime`

`useRealtime(tables, callback)` subscribes to realtime events and cleans up automatically.

```tsx
import { useRealtime } from "@allyourbase/react";

export function RealtimeFeed() {
  useRealtime(["posts", "comments"], (event) => {
    console.log(event);
  });

  return <p>Watching posts/comments...</p>;
}
```

## Accessing the raw client

```tsx
import { AYBClient } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function ApiKeyMode() {
  const ayb = useAYBClient() as AYBClient;
  ayb.setApiKey("ayb_api_key_xxx");
  return null;
}
```

## Related guides

- [JavaScript SDK](/guide/javascript-sdk)
- [Realtime](/guide/realtime)
- [Authentication](/guide/authentication)
