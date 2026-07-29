# @allyourbase/js

JavaScript/TypeScript client SDK for [Allyourbase](https://github.com/gridlhq-staging/allyourbase) — the PostgreSQL Backend-as-a-Service.

## Install

```bash
npm install @allyourbase/js
```

The published npm package currently resolves at version `0.2.0`.

## Quick Start

```ts
import { AYBClient, type SearchHit } from "@allyourbase/js";

const ayb = new AYBClient("http://localhost:8090");

// Create a record
const post = await ayb.records.create("posts", {
  title: "Hello World",
  published: true,
});

// List records with filtering and sorting
const posts = await ayb.records.list("posts", {
  filter: "published=true",
  sort: "-created_at",
  perPage: 20,
});

// Search records with highlights and facet counts
const search = await ayb.records.list<SearchHit<{ id: string; title: string }>>(
  "posts",
  {
    search: "postgres",
    fuzzy: true,
    typoThreshold: 0.3,
    highlight: true,
    facets: ["published"],
  },
);
console.log(search.items[0]?._highlight, search.facets?.published);

// Auth
await ayb.auth.login("user@example.com", "password");
const me = await ayb.auth.me();

// Passkey sign-in and MFA
await ayb.auth.signInWithPasskey("user@example.com");
await ayb.auth.enrollPasskey("Primary passkey");
```

`highlight` is a boolean toggle that asks the backend to return `_highlight`
snippets on matching items. `typoThreshold` is only accepted when `fuzzy: true`.

## API Reference

### `new AYBClient(baseURL, options?)`

Create a client instance.

```ts
const ayb = new AYBClient("http://localhost:8090");

// With custom fetch (e.g. for Node.js < 18)
const ayb = new AYBClient("http://localhost:8090", { fetch: myFetch });
```

### PostgreSQL RPC

Call PostgreSQL functions with
`client.rpc<T>(functionName, args?, options?)`. Void and empty responses resolve
to `undefined`.

```ts
const total = await ayb.rpc<number>("leaderboard_total", {
  club_id: "abc123",
});
```

### Edge functions

`client.functions.invoke(name, options?)` returns the raw response as
`{ status, headers, rawBody }`.

```ts
const response = await ayb.functions.invoke("send-digest", {
  body: { club_id: "abc123" },
});
console.log(response.status, response.headers, response.rawBody);
```

GraphQL and admin/org typed clients are JS-only scope today and live on
`client.graphql` and `client.admin(...)`. The other SDKs intentionally do not
define those typed clients yet.

### Records

```ts
// List with filtering, sorting, pagination
const result = await ayb.records.list<Post>("posts", {
  filter: "status='active' AND views>100",
  sort: "-created_at,+title",
  page: 1,
  perPage: 50,
  fields: "id,title,status",
  expand: "author,category",
  skipTotal: true,
});
// result: { items: Post[], page, perPage, totalItems, totalPages }

// Search with typo tolerance, highlights, and facets
const search = await ayb.records.list<SearchHit<Post>>("posts", {
  search: "postgres database",
  fuzzy: true,
  typoThreshold: 0.3,
  highlight: true,
  facets: ["status", "category"],
});
// search.items[0]._highlight is present when the backend returns a highlight.
// search.facets is a FacetCounts envelope keyed by requested facet column.

// Semantic/vector search
await ayb.records.list<Post>("posts", {
  semantic: true,
  semanticQuery: "articles about hosted Postgres",
  nearest: [0.12, 0.34, 0.56],
  vectorColumn: "embedding",
  distance: "cosine",
});

// Get by ID
const post = await ayb.records.get<Post>("posts", "abc123", {
  expand: "author",
});

// Create
const post = await ayb.records.create<Post>("posts", {
  title: "New Post",
  body: "Content here",
});

// Update (partial)
const updated = await ayb.records.update<Post>("posts", "abc123", {
  title: "Updated Title",
});

// Delete
await ayb.records.delete("posts", "abc123");
```

`highlight` mirrors the REST API's boolean query param: pass `true` to request
`_highlight` snippets, and omit it otherwise. `typoThreshold` requires
`fuzzy: true`.

### InstantSearch adapter

```ts
import { createInstantSearchClient } from "@allyourbase/js/instantsearch";

const searchClient = createInstantSearchClient({
  client: ayb,
  objectIDField: "id",
  defaultIndexName: "posts",
});
```

`@allyourbase/js/instantsearch` is a thin adapter over `records.list`; it does
not add a second search transport. `objectIDField` is required because AYB rows
are arbitrary PostgreSQL records, and the adapter fails closed when a returned
row is missing that field or has a `null` value.

The adapter supports one-index `search(requests)` calls with `query`, zero-based
`page`, `hitsPerPage`, concrete `facets`, `disjunctiveFacets`,
`facetFilters` in `attribute:value` form, `numericFilters` range comparisons,
and the documented `filters` comparison subset. Empty query strings are sent as
browsable list calls with no `search` parameter so first-render facets and
range stats remain available. Set `highlight: false` in the adapter options to
omit the backend `highlight=true` request flag.

`searchClient.searchForFacetValues(requests)` is supported for searchable facet
widgets. It delegates each request through `client.records.searchFacetValues()`
(see below) and returns Algolia-shaped `{ facetHits, exhaustiveFacetsCount,
processingTimeMS }` per request; the backend's `<mark>` prefix wrappers are
remapped onto the caller's `highlightPreTag`/`highlightPostTag` (default
InstantSearch placeholders). `maxFacetHits` defaults to 10 and is capped at
100.

Unsupported cases throw before AYB is called: mixed index requests,
wildcard facets, vector/search tuning params, `skipTotal`, negative
`facetFilters`, nested attributes, tag filters, malformed numeric filters,
arrays, `NOT`, and unlisted Algolia request parameters.

### Facet value search

```ts
// Searchable facet values: bucket-level search on a single facet column
const facet = await ayb.records.searchFacetValues("products", "category", {
  q: "st",
  maxFacetHits: 10,
});
// facet.facetHits[0]: { value: "Stationery", highlighted: "<mark>St</mark>ationery", count: 3 }
console.log(facet.exhaustiveFacetsCount);
```

`records.searchFacetValues(collection, column, params)` calls
`GET /api/collections/{table}/facets/{column}/search`. `column` must be a text
facet column. `params` accepts an optional `q` prefix, `maxFacetHits`,
`filter`, and `search` (the same scoping predicates the list endpoint accepts).
`maxFacetHits` defaults to 10 and is capped at 100.
The InstantSearch adapter's
`searchForFacetValues(requests)` uses this method as its transport.

### Auth

```ts
// Register
const { token, refreshToken, user } = await ayb.auth.register(
  "user@example.com",
  "password123",
);

// Login
await ayb.auth.login("user@example.com", "password123");

// Current user
const me = await ayb.auth.me();

// Refresh token
await ayb.auth.refresh();

// Logout
await ayb.auth.logout();

// Password reset
await ayb.auth.requestPasswordReset("user@example.com");
await ayb.auth.confirmPasswordReset(token, "newpassword");

// Email verification
await ayb.auth.verifyEmail(token);
await ayb.auth.resendVerification();

// First-factor WebAuthn login
const challenge = await ayb.auth.beginWebAuthnLogin("user@example.com");
await ayb.auth.finishWebAuthnLogin(challenge.challengeId, assertionResponse);

// Browser passkey convenience flow
await ayb.auth.signInWithPasskey("user@example.com");

// WebAuthn MFA enrollment and verification
await ayb.auth.enrollPasskey("Work laptop");
await ayb.auth.verifyPasskey(mfaToken);

// Restore tokens from storage
ayb.setTokens(savedToken, savedRefreshToken);
```

### Storage

```ts
// Upload a file to a bucket
const file = document.querySelector("input[type=file]").files[0];
const result = await ayb.storage.upload("avatars", file);
// result: { id, bucket, name, size, contentType, createdAt, updatedAt }

// Upload with a custom filename
await ayb.storage.upload("documents", blob, "report.pdf");

// Get download URL
const url = ayb.storage.downloadURL("avatars", "photo.jpg");
// → "http://localhost:8090/api/storage/avatars/photo.jpg"

// List files in a bucket
const files = await ayb.storage.list("avatars", { prefix: "user_", limit: 20 });

// Get a signed URL (time-limited access, default 1 hour)
const { url: signedUrl } = await ayb.storage.getSignedURL("avatars", "photo.jpg", 3600);

// Delete
await ayb.storage.delete("avatars", "photo.jpg");
```

### Realtime

```ts
// Subscribe to table changes (Server-Sent Events)
const unsubscribe = ayb.realtime.subscribe(
  ["posts", "comments"],
  (event) => {
    console.log(event.action, event.table, event.record);
    // action: "create" | "update" | "delete"
  },
);

// Stop listening
unsubscribe();
```

## TypeScript

All methods accept generic type parameters for full type safety:

```ts
interface Post {
  id: string;
  title: string;
  published: boolean;
  created_at: string;
}

const posts = await ayb.records.list<Post>("posts");
// posts.items is Post[]
```

Exported types include:

`AdminAPIKey`, `AdminAPIKeyListResponse`, `App`, `AppListResponse`, `AuthPersistence`, `AuthResponse`, `AuthStateEvent`, `AuthStateListener`, `BatchOperation`, `BatchResult`, `ClientOptions`, `CreateAdminAPIKeyRequest`, `CreateAdminAPIKeyResponse`, `CreateOAuthClientRequest`, `CreateOAuthClientResponse`, `FacetCounts`, `FacetValueCount`, `FacetValueSearchHit`, `FacetValueSearchParams`, `FacetValueSearchResponse`, `GetParams`, `GraphQLErrorItem`, `GraphQLResponse`, `HealthResponse`, `ListParams`, `ListResponse`, `MagicLinkConfirmResponse`, `MagicLinkRequestResponse`, `MFAPendingAuthResponse`, `OAuthClient`, `OAuthClientListResponse`, `OAuthOptions`, `OAuthProvider`, `OAuthTokenResponse`, `PersistedAuthSession`, `PublicKeyCredentialCreationOptionsJSON`, `PublicKeyCredentialDescriptorJSON`, `PublicKeyCredentialParametersJSON`, `PublicKeyCredentialRequestOptionsJSON`, `PublicKeyCredentialRpEntityJSON`, `PublicKeyCredentialUserEntityJSON`, `RealtimeEvent`, `RotateOAuthClientSecretResponse`, `RpcNotifyOption`, `RpcOptions`, `SearchHit`, `StorageObject`, `UpdateOAuthClientRequest`, `User`, `WebAuthnEnrollBeginResponse`, `WebAuthnEnrollConfirmRequest`, `WebAuthnLoginBeginResponse`, `WebAuthnLoginFinishRequest`, `WebAuthnMFAChallengeResponse`, and `WebAuthnMFAVerifyRequest`.

## Error Handling

All API errors throw `AYBError` with the HTTP status code:

```ts
import { AYBClient, AYBError } from "@allyourbase/js";

try {
  await ayb.records.get("posts", "nonexistent");
} catch (err) {
  if (err instanceof AYBError) {
    console.log(err.status);  // 404
    console.log(err.message); // "record not found"
  }
}
```

## License

MIT
