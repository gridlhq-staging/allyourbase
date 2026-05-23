# @allyourbase/react

React integration helpers for Allyourbase.

## Quick start

```tsx
import { AYBClient } from "@allyourbase/js";
import { AYBProvider, useQuery } from "@allyourbase/react";

const client = new AYBClient("http://localhost:8090");

function Posts() {
  const { data, loading } = useQuery("posts");
  if (loading) return <div>Loading...</div>;
  return <pre>{JSON.stringify(data?.items)}</pre>;
}

export function App() {
  return (
    <AYBProvider client={client}>
      <Posts />
    </AYBProvider>
  );
}
```

## Auth hooks

`useAuth()` exposes the shared client auth helpers, including anonymous sign-in,
magic-link request/confirm, and email-link upgrade flows.

```tsx
import { useAuth } from "@allyourbase/react";

function AuthActions() {
  const { user, signInAnonymously, requestMagicLink, confirmMagicLink, linkEmail } = useAuth();

  return (
    <>
      <button onClick={() => void signInAnonymously()}>Continue as guest</button>
      <button onClick={() => void requestMagicLink("user@example.com")}>Send magic link</button>
      <button onClick={() => void confirmMagicLink("token-from-email")}>Confirm magic link</button>
      <button onClick={() => void linkEmail("upgraded@example.com", "StrongPass123!")}>Link email</button>
      <pre>{JSON.stringify(user)}</pre>
    </>
  );
}
```
