/** List response envelope returned by collection endpoints. */
export interface ListResponse<T = Record<string, unknown>> {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

/** Parameters for listing records. */
export interface ListParams {
  page?: number;
  perPage?: number;
  sort?: string;
  filter?: string;
  search?: string;
  fields?: string;
  expand?: string;
  skipTotal?: boolean;
}

/** Parameters for reading a single record. */
export interface GetParams {
  fields?: string;
  expand?: string;
}

/** Auth tokens returned by login/register. */
export interface AuthResponse {
  token: string;
  refreshToken: string;
  user: User;
}

/** User record from the auth system. */
export interface User {
  id: string;
  email: string;
  emailVerified?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/** Realtime event from SSE stream. */
export interface RealtimeEvent {
  action: "create" | "update" | "delete";
  table: string;
  record: Record<string, unknown>;
}

/** Stored file metadata returned by storage endpoints. */
export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  userId?: string;
  createdAt: string;
  updatedAt: string;
}

/** A single operation within a batch request. */
export interface BatchOperation {
  method: "create" | "update" | "delete";
  id?: string;
  body?: Record<string, unknown>;
}

/** Result of a single operation within a batch response. */
export interface BatchResult<T = Record<string, unknown>> {
  index: number;
  status: number;
  body?: T;
}

/** Client configuration options. */
export interface ClientOptions {
  /** Custom fetch implementation (e.g. for Node.js < 18). */
  fetch?: typeof globalThis.fetch;
}

/** Supported OAuth providers. */
export type OAuthProvider = "google" | "github";

/** Options for the `signInWithOAuth()` method. */
export interface OAuthOptions {
  /** Additional scopes to request from the OAuth provider. */
  scopes?: string[];
  /**
   * Custom URL handler for redirect-based flow (instead of popup).
   * When provided, no popup is opened — the SDK calls this with the
   * authorization URL so the app can redirect.
   * Use this for iOS PWAs or when popups are blocked.
   */
  urlCallback?: (url: string) => void | Promise<void>;
}

/** Auth state change events emitted by `onAuthStateChange`. */
export type AuthStateEvent = "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED";

/** Callback for auth state change events. */
export type AuthStateListener = (
  event: AuthStateEvent,
  session: { token: string; refreshToken: string } | null,
) => void;
