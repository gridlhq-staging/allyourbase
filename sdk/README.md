# @allyourbase/js

JavaScript/TypeScript client SDK for [Allyourbase](https://github.com/gridlhq-staging/allyourbase) — the PostgreSQL Backend-as-a-Service.

## Install

```bash
npm install @allyourbase/js
```

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
    highlight: "title",
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

## API Reference

### `new AYBClient(baseURL, options?)`

Create a client instance.

```ts
const ayb = new AYBClient("http://localhost:8090");

// With custom fetch (e.g. for Node.js < 18)
const ayb = new AYBClient("http://localhost:8090", { fetch: myFetch });
```

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
  highlight: "title",
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

`AdminAPIKey`, `AdminAPIKeyListResponse`, `App`, `AppListResponse`, `AuthPersistence`, `AuthResponse`, `AuthStateEvent`, `AuthStateListener`, `BatchOperation`, `BatchResult`, `ClientOptions`, `CreateAdminAPIKeyRequest`, `CreateAdminAPIKeyResponse`, `CreateOAuthClientRequest`, `CreateOAuthClientResponse`, `FacetCounts`, `FacetValueCount`, `GetParams`, `GraphQLErrorItem`, `GraphQLResponse`, `HealthResponse`, `ListParams`, `ListResponse`, `MagicLinkConfirmResponse`, `MagicLinkRequestResponse`, `MFAPendingAuthResponse`, `OAuthClient`, `OAuthClientListResponse`, `OAuthOptions`, `OAuthProvider`, `OAuthTokenResponse`, `PersistedAuthSession`, `PublicKeyCredentialCreationOptionsJSON`, `PublicKeyCredentialDescriptorJSON`, `PublicKeyCredentialParametersJSON`, `PublicKeyCredentialRequestOptionsJSON`, `PublicKeyCredentialRpEntityJSON`, `PublicKeyCredentialUserEntityJSON`, `RealtimeEvent`, `RotateOAuthClientSecretResponse`, `RpcNotifyOption`, `RpcOptions`, `SearchHit`, `StorageObject`, `UpdateOAuthClientRequest`, `User`, `WebAuthnEnrollBeginResponse`, `WebAuthnEnrollConfirmRequest`, `WebAuthnLoginBeginResponse`, `WebAuthnLoginFinishRequest`, `WebAuthnMFAChallengeResponse`, and `WebAuthnMFAVerifyRequest`.

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
