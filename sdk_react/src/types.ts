/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/jun01_pm_2_passwordless_passkey_login/allyourbase_dev/sdk_react/src/types.ts.
 */
export type AuthStateEvent = "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED";
export type OAuthProvider = "google" | "github";

export interface OAuthOptions {
  scopes?: string[];
  urlCallback?: (url: string) => void | Promise<void>;
  /**
   * Per-request OAuth post-callback redirect target. Server validates against
   * `AYB_AUTH_OAUTH_RETURN_TO_ALLOWLIST` at both start and callback dispatch
   * (see `internal/auth/handler_oauth.go`). React just forwards the value to
   * the underlying JS client; the server is the single security owner.
   */
  redirectTo?: string;
}

export type AuthStateListener = (
  event: AuthStateEvent,
  session: { token: string; refreshToken: string } | null,
) => void;

export interface UserLike {
  id: string;
  email?: string;
  phone?: string;
  isAnonymous?: boolean;
  linkedAt?: string;
  emailVerified?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface RealtimeEventLike {
  action: "create" | "update" | "delete" | "INSERT" | "UPDATE" | "DELETE";
  table: string;
  record: Record<string, unknown>;
  oldRecord?: Record<string, unknown>;
}

/**
 * Client interface providing authentication, records access, and real-time subscriptions. Maintains auth state through token and refreshToken properties. The auth namespace includes login, register, logout, refresh, and me operations; records provides data querying; realtime enables table subscriptions. Register listeners for authentication state changes with onAuthStateChange, which returns an unsubscribe function.
 */
export interface AYBClientLike {
  token: string | null;
  refreshToken: string | null;
  clearTokens?: () => void;
  waitForSessionRestore?: () => Promise<void>;
  onAuthStateChange(listener: AuthStateListener): () => void;
  auth: {
    login(email: string, password: string): Promise<unknown>;
    register(email: string, password: string): Promise<unknown>;
    signInAnonymously(): Promise<unknown>;
    signInWithPasskey?(email: string): Promise<unknown>;
    requestMagicLink(email: string): Promise<unknown>;
    confirmMagicLink(token: string): Promise<unknown>;
    linkEmail(email: string, password: string): Promise<unknown>;
    signInWithOAuth(provider: OAuthProvider, options?: OAuthOptions): Promise<unknown>;
    logout(): Promise<void>;
    refresh(): Promise<unknown>;
    me(): Promise<UserLike>;
  };
  records: {
    list<T = Record<string, unknown>>(
      collection: string,
      params?: Record<string, unknown>,
    ): Promise<{ items: T[]; page: number; perPage: number; totalItems: number; totalPages: number }>;
  };
  realtime: {
    subscribe(
      tables: string[],
      callback: (event: RealtimeEventLike) => void,
    ): () => void;
  };
}

export interface UseQueryOptions {
  enabled?: boolean;
  suspense?: boolean;
}

export interface UseQueryResult<T> {
  data: { items: T[]; page: number; perPage: number; totalItems: number; totalPages: number } | null;
  loading: boolean;
  error: Error | null;
  refetch: () => Promise<void>;
}

export interface UseAuthResult {
  loading: boolean;
  user: UserLike | null;
  error: Error | null;
  token: string | null;
  refreshToken: string | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string) => Promise<void>;
  signInAnonymously: () => Promise<void>;
  signInWithPasskey: (email: string) => Promise<void>;
  requestMagicLink: (email: string) => Promise<void>;
  confirmMagicLink: (token: string) => Promise<void>;
  linkEmail: (email: string, password: string) => Promise<void>;
  signInWithOAuth: (provider: OAuthProvider, options?: OAuthOptions) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
}

export interface DemoSuggestion {
  label: string;
  email: string;
  password: string;
}

export interface AybAuthMethods {
  password: boolean;
  oauth: boolean;
  anonymous: boolean;
  canUpgradeAnonymous: boolean;
  magicLink?: boolean;
  passkey?: boolean;
}

export interface AybLoginBarProps {
  methods: AybAuthMethods;
  loading: boolean;
  mode?: "login" | "register";
  submitLabel?: string;
  registerToggleLabel?: string;
  loginToggleLabel?: string;
  email: string;
  emailPlaceholder?: string;
  password: string;
  passwordPlaceholder?: string;
  error: string | null;
  demoSuggestions: DemoSuggestion[];
  oauthProviders?: OAuthProvider[];
  onEmailChange: (value: string) => void;
  onPasswordChange: (value: string) => void;
  onModeChange?: (mode: "login" | "register") => void;
  onSubmit: () => Promise<void>;
  onOAuth: () => Promise<void>;
  onAnonymous: () => Promise<void>;
  onOAuthProvider?: (provider: OAuthProvider) => Promise<void>;
  onPasskey?: (email: string) => Promise<void>;
  onRequestMagicLink?: (email: string) => Promise<void>;
  onUpgradeAnonymous?: () => Promise<void>;
}
