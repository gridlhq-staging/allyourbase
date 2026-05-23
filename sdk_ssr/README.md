# @allyourbase/ssr

Server-side cookie/session helpers for Allyourbase.

## Quick start

```ts
import { loadServerSession } from "@allyourbase/ssr";

const result = await loadServerSession({
  cookieHeader: request.headers.get("cookie") ?? "",
  client: ayb,
});
```

## Magic-link confirmation

`confirmMagicLinkServer()` exchanges a magic-link token and returns the session
plus any `Set-Cookie` headers that should be forwarded by your framework.

```ts
import { confirmMagicLinkServer } from "@allyourbase/ssr";

const result = await confirmMagicLinkServer({
  client: ayb,
  token: searchParams.get("token") ?? "",
});

for (const header of result.setCookieHeaders) {
  response.headers.append("Set-Cookie", header);
}
```

If the server responds with pending MFA, `session` is `null` and
`setCookieHeaders` is empty so your route can redirect into the MFA flow.
