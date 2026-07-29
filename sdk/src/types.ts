/** Single facet bucket returned inside the facets envelope. */
export interface FacetValueCount {
  value: unknown;
  count: number;
}

/** Facet counts keyed by column name, matching backend FacetCounts. */
export type FacetCounts = Record<string, FacetValueCount[]>;

/** List response envelope returned by collection endpoints. */
export interface ListResponse<T = Record<string, unknown>> {
  items: T[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
  facets?: FacetCounts;
  facetStats?: Record<string, { min: string | number; max: string | number }>;
}

/** Per-attribute highlight metadata returned when callers pass `highlight=true`. */
export interface SearchHighlightResultEntry {
  value: string;
  matchLevel: "full" | "none" | string;
}

/** Highlight metadata keyed by searchable attribute name. */
export type SearchHighlightResult = Record<string, SearchHighlightResultEntry>;

/**
 * Mixin that widens an item shape with optional highlight fields the backend
 * search path returns when callers pass the `highlight` query param. `_highlight`
 * is the legacy combined excerpt; `_highlightResult` is keyed by searchable
 * attribute and includes each attribute's highlighted value plus match level.
 * Used as the default item shape for `RecordsClient.list` so untyped callers
 * see highlight metadata without a cast, and as an explicit wrapper for typed
 * callers (e.g. `list<SearchHit<MyRow>>(...)`).
 */
export type SearchHit<T = Record<string, unknown>> = T & {
  _highlight?: string;
  _highlightResult?: SearchHighlightResult;
};

/** Single GraphQL error payload item from the GraphQL `errors` envelope. */
export interface GraphQLErrorItem {
  message: string;
  [key: string]: unknown;
}

/** GraphQL JSON response envelope. */
export interface GraphQLResponse<TData> {
  data?: TData;
  errors?: GraphQLErrorItem[];
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
  fuzzy?: boolean;
  typoThreshold?: number;
  highlight?: boolean;
  facets?: string[];
  disjunctiveFacets?: string[];
  semantic?: boolean;
  semanticQuery?: string;
  nearest?: number[];
  vectorColumn?: string;
  distance?: string;
}

/** Parameters for reading a single record. */
export interface GetParams {
  fields?: string;
  expand?: string;
}

/**
 * Single bucket returned by the facet-value-search endpoint. The route only
 * supports text facet columns, so `value` is always a non-null string.
 * `highlighted` wraps the matched prefix in `<mark>...</mark>` on the server.
 */
export interface FacetValueSearchHit {
  value: string;
  highlighted: string;
  count: number;
}

/** Response envelope for GET /api/collections/{table}/facets/{column}/search. */
export interface FacetValueSearchResponse {
  facetHits: FacetValueSearchHit[];
  exhaustiveFacetsCount: boolean;
}

/**
 * Parameters for `RecordsClient.searchFacetValues`. `filter` and `search` mirror
 * the same-named fields on `ListParams` so callers can reuse list query strings.
 */
export interface FacetValueSearchParams {
  q?: string;
  maxFacetHits?: number;
  filter?: string;
  search?: string;
}

/** Auth tokens returned by login/register. */
export interface AuthResponse {
  token: string;
  refreshToken: string;
  user: User;
}

/** Successful response body for POST /api/auth/magic-link. */
export interface MagicLinkRequestResponse {
  message: string;
}

/** MFA challenge response body when magic-link confirm requires second factor. */
export interface MFAPendingAuthResponse {
  mfaPending: true;
  mfaToken: string;
}

/** Response body for POST /api/auth/magic-link/confirm. */
export type MagicLinkConfirmResponse = AuthResponse | MFAPendingAuthResponse;

/** JSON-serializable credential descriptor used by WebAuthn request options. */
export interface PublicKeyCredentialDescriptorJSON {
  id: string;
  type: PublicKeyCredentialType;
  transports?: AuthenticatorTransport[];
}

/** JSON-serializable request options for first-factor WebAuthn login. */
export interface PublicKeyCredentialRequestOptionsJSON {
  challenge: string;
  timeout?: number;
  rpId?: string;
  allowCredentials?: PublicKeyCredentialDescriptorJSON[];
  userVerification?: UserVerificationRequirement;
  extensions?: AuthenticationExtensionsClientInputs;
}

/** Response body for POST /api/auth/webauthn/login/begin. */
export interface WebAuthnLoginBeginResponse {
  challengeId: string;
  options: PublicKeyCredentialRequestOptionsJSON;
}

/** Request body for POST /api/auth/webauthn/login/finish in canonical SDK shape. */
export interface WebAuthnLoginFinishRequest {
  challengeId: string;
  assertionResponse: Record<string, unknown>;
}

/** Public passkey credential metadata returned by credential-management helpers. */
export interface PasskeyCredentialMetadata {
  credentialId: string;
  displayName: string;
  transports: string[];
  createdAt: string;
  lastUsedAt?: string;
}

/** Response envelope for GET /api/auth/mfa/webauthn/credentials in canonical SDK shape. */
export interface PasskeyCredentialListResponse {
  credentials: PasskeyCredentialMetadata[];
}

/** JSON-serializable PublicKeyCredentialUserEntity used by WebAuthn creation options. */
export interface PublicKeyCredentialUserEntityJSON {
  id: string;
  name: string;
  displayName: string;
}

/** JSON-serializable RP entity used by WebAuthn creation options. */
export interface PublicKeyCredentialRpEntityJSON {
  id?: string;
  name: string;
}

/** Single pubKeyCredParam entry in serialized creation options. */
export interface PublicKeyCredentialParametersJSON {
  type: PublicKeyCredentialType;
  alg: number;
}

/**
 * JSON-serializable creation options for WebAuthn enrollment.
 * Mirrors go-webauthn's serialized PublicKeyCredentialCreationOptions:
 * `challenge`, `user.id`, and each `excludeCredentials[].id` are base64url
 * strings that the SDK decodes to ArrayBuffers before navigator.credentials.create.
 */
export interface PublicKeyCredentialCreationOptionsJSON {
  challenge: string;
  rp: PublicKeyCredentialRpEntityJSON;
  user: PublicKeyCredentialUserEntityJSON;
  pubKeyCredParams: PublicKeyCredentialParametersJSON[];
  timeout?: number;
  excludeCredentials?: PublicKeyCredentialDescriptorJSON[];
  authenticatorSelection?: AuthenticatorSelectionCriteria;
  attestation?: AttestationConveyancePreference;
  extensions?: AuthenticationExtensionsClientInputs;
}

/**
 * Response body from POST /api/auth/mfa/webauthn/enroll.
 * The backend writes `creation.Response` directly (go-webauthn's
 * `PublicKeyCredentialCreationOptions` value), so the JSON body IS the
 * creation options — there is no outer `{publicKey: ...}` wrapper.
 */
export type WebAuthnEnrollBeginResponse = PublicKeyCredentialCreationOptionsJSON;

/** Request body for POST /api/auth/mfa/webauthn/enroll/confirm in canonical SDK shape. */
export interface WebAuthnEnrollConfirmRequest {
  displayName: string;
  attestationResponse: Record<string, unknown>;
}

/** Response body for POST /api/auth/mfa/webauthn/challenge. */
export interface WebAuthnMFAChallengeResponse {
  challengeId: string;
  options: PublicKeyCredentialRequestOptionsJSON;
}

/** Request body for POST /api/auth/mfa/webauthn/verify in canonical SDK shape. */
export interface WebAuthnMFAVerifyRequest {
  challengeId: string;
  assertionResponse: Record<string, unknown>;
}

/** Health check response returned by GET /health. */
export interface HealthResponse {
  status: string;
  database: string;
}

/** User record from the auth system. */
export interface User {
  id: string;
  email?: string;
  phone?: string;
  isAnonymous?: boolean;
  linkedAt?: string;
  emailVerified?: boolean;
  createdAt?: string;
  updatedAt?: string;
}

/** Registered app (matches admin apps API response). */
export interface App {
  id: string;
  name: string;
  description: string;
  ownerUserId: string;
  rateLimitRps: number;
  rateLimitWindowSeconds: number;
  createdAt: string;
  updatedAt: string;
}

/** Paginated app list envelope returned by admin apps API. */
export type AppListResponse = ListResponse<App>;

/** Admin API key record (matches admin api-keys API response). */
export interface AdminAPIKey {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  scope: string;
  allowedTables: string[] | null;
  appId: string | null;
  lastUsedAt: string | null;
  expiresAt: string | null;
  createdAt: string;
  revokedAt: string | null;
}

/** Paginated admin API key list envelope. */
export type AdminAPIKeyListResponse = ListResponse<AdminAPIKey>;

/** Organization membership roles accepted by org-admin membership routes. */
export type OrgRole = "owner" | "admin" | "member" | "viewer";

/** Team membership roles accepted by org-admin team membership routes. */
export type TeamRole = "lead" | "member";

/** Organization record returned by org-admin organization routes. */
export interface Organization {
  id: string;
  name: string;
  slug: string;
  parentOrgId?: string | null;
  planTier: string;
  createdAt: string;
  updatedAt: string;
}

/** Detailed organization response with server-owned child resource counts. */
export interface OrganizationDetail extends Organization {
  childOrgCount: number;
  teamCount: number;
  tenantCount: number;
}

/** Team record returned by org-admin team routes. */
export interface Team {
  id: string;
  orgId: string;
  name: string;
  slug: string;
  createdAt: string;
  updatedAt: string;
}

/** Organization membership record returned by org-admin member routes. */
export interface OrgMembership {
  id: string;
  orgId: string;
  userId: string;
  role: OrgRole;
  createdAt: string;
}

/** Team membership record returned by org-admin team member routes. */
export interface TeamMembership {
  id: string;
  teamId: string;
  userId: string;
  role: TeamRole;
  createdAt: string;
}

/** Tenant record returned by org-admin tenant assignment list routes. */
export interface OrgTenant {
  id: string;
  name: string;
  slug: string;
  isolationMode: string;
  planTier: string;
  region: string;
  orgId?: string | null;
  orgMetadata: Record<string, unknown>;
  state: "provisioning" | "active" | "suspended" | "deleting" | "deleted";
  idempotencyKey?: string | null;
  createdAt: string;
  updatedAt: string;
}

/** Request body for creating an organization through org-admin routes. */
export interface CreateOrganizationRequest {
  name: string;
  slug: string;
  parentOrgId?: string | null;
  planTier: string;
}

/** Request body for updating organization metadata through org-admin routes. */
export interface UpdateOrganizationRequest {
  name?: string;
  slug?: string;
  /**
   * Omit to leave the parent unchanged. The server treats JSON null as an
   * omitted update, so use the empty string "" — the server-owned removal
   * sentinel — to detach the organization from its parent.
   */
  parentOrgId?: string;
}

/** Request body for creating a team through org-admin routes. */
export interface CreateTeamRequest {
  name: string;
  slug: string;
}

/** Request body for updating a team through org-admin routes. */
export interface UpdateTeamRequest {
  name?: string;
  slug?: string;
}

/** Request body for adding an organization member. */
export interface AddOrgMemberRequest {
  userId: string;
  role: OrgRole;
}

/** Request body for updating an organization member role. */
export interface UpdateOrgMemberRoleRequest {
  role: OrgRole;
}

/** Request body for adding a team member. */
export interface AddTeamMemberRequest {
  userId: string;
  role: TeamRole;
}

/** Request body for updating a team member role. */
export interface UpdateTeamMemberRoleRequest {
  role: TeamRole;
}

/** Request body for assigning an existing tenant to an organization. */
export interface AssignOrgTenantRequest {
  tenantId: string;
}

/** Response body returned when a tenant assignment succeeds. */
export interface AssignOrgTenantResponse {
  status: "assigned";
}

/** Required acknowledgement for destructive org deletion. */
export interface DeleteOrganizationOptions {
  confirm: true;
}

/** Organization list envelope returned by org-admin list routes. */
export interface OrganizationListResponse {
  items: Organization[];
}

/** Team list envelope returned by org-admin team list routes. */
export interface TeamListResponse {
  items: Team[];
}

/** Organization member list envelope returned by org-admin member list routes. */
export interface OrgMembershipListResponse {
  items: OrgMembership[];
}

/** Team member list envelope returned by org-admin team member list routes. */
export interface TeamMembershipListResponse {
  items: TeamMembership[];
}

/** Tenant assignment list envelope returned by org-admin tenant list routes. */
export interface OrgTenantListResponse {
  items: OrgTenant[];
}

/** One daily usage bucket returned by org-admin usage routes. */
export interface OrgUsageDayEntry {
  date: string;
  apiRequests: number;
  storageBytesUsed: number;
  bandwidthBytes: number;
  functionInvocations: number;
}

/** Aggregated usage totals returned by org-admin usage routes. */
export interface OrgUsageTotals {
  apiRequests: number;
  storageBytesUsed: number;
  bandwidthBytes: number;
  functionInvocations: number;
}

/** Optional query params accepted by org-admin usage routes. */
export interface OrgUsageOptions {
  period?: "day" | "week" | "month";
  from?: string;
  to?: string;
}

/** Usage summary returned by org-admin usage routes. */
export interface OrgUsageSummary {
  orgId: string;
  tenantCount: number;
  period: string;
  data: OrgUsageDayEntry[];
  totals: OrgUsageTotals;
}

/** Optional query params accepted by org-admin audit routes. */
export interface OrgAuditOptions {
  from?: string;
  to?: string;
  action?: string;
  result?: string;
  actorId?: string;
  limit?: number;
  offset?: number;
}

/** Tenant audit event returned by org-admin audit routes. */
export interface OrgAuditEvent {
  id: string;
  tenantId: string;
  actorId?: string | null;
  action: string;
  result: string;
  metadata: Record<string, unknown> | null;
  ipAddress?: string | null;
  createdAt: string;
}

/** Audit list envelope returned by org-admin audit routes. */
export interface OrgAuditListResponse {
  items: OrgAuditEvent[];
  count: number;
  limit: number;
  offset: number;
}

/** Request body for creating an admin API key. */
export interface CreateAdminAPIKeyRequest {
  userId: string;
  name: string;
  scope?: string;
  allowedTables?: string[];
  appId?: string;
}

/** Response body when an admin API key is created. */
export interface CreateAdminAPIKeyResponse {
  key: string;
  apiKey: AdminAPIKey;
}

/** Realtime event from SSE stream. */
export interface RealtimeEvent {
  action: "create" | "update" | "delete" | "INSERT" | "UPDATE" | "DELETE";
  table: string;
  record: Record<string, unknown>;
  oldRecord?: Record<string, unknown>;
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
  updatedAt?: string;
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

/** Token pair persisted outside the client for auth session restore. */
export interface PersistedAuthSession {
  token: string;
  refreshToken: string;
}

/** Optional persistence callbacks for best-effort auth session storage. */
export interface AuthPersistence {
  load?: () => PersistedAuthSession | null | Promise<PersistedAuthSession | null>;
  save?: (session: PersistedAuthSession) => void | Promise<void>;
  clear?: () => void | Promise<void>;
}

/** Client configuration options. */
export interface ClientOptions {
  /** Custom fetch implementation (e.g. for Node.js < 18). */
  fetch?: typeof globalThis.fetch;
  /** Optional auth session persistence callbacks. */
  authPersistence?: AuthPersistence;
}

/** Notify metadata sent with RPC requests to trigger realtime events. */
export interface RpcNotifyOption {
  table: string;
  action: "create" | "update" | "delete";
}

/** Optional RPC transport options mirrored from backend request headers. */
export interface RpcOptions {
  notify?: RpcNotifyOption;
}

/** Options for invoking an edge function via `functions.invoke`. */
export interface EdgeInvokeOptions {
  /** Request body. Plain objects are JSON-serialized; strings and binary BodyInit values are sent verbatim. */
  body?: unknown;
  /** Additional request headers merged over the client defaults. */
  headers?: Record<string, string>;
  /** HTTP method; defaults to POST. */
  method?: string;
  /** Skip attaching the client's bearer token. */
  skipAuth?: boolean;
}

/** Raw edge function response envelope returned by `functions.invoke`. */
export interface EdgeInvokeResponse {
  /** HTTP status code. */
  status: number;
  /** Response headers as a plain record. */
  headers: Record<string, string>;
  /** Raw response body text; empty string for a 204 No Content response. */
  rawBody: string;
}

/** Registered OAuth client (matches admin OAuth clients API response). */
export interface OAuthClient {
  id: string;
  appId: string;
  clientId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType: "confidential" | "public";
  createdAt: string;
  updatedAt: string;
  revokedAt: string | null;
  activeAccessTokenCount: number;
  activeRefreshTokenCount: number;
  totalGrants: number;
  lastTokenIssuedAt: string | null;
}

/** Paginated OAuth client list envelope. */
export type OAuthClientListResponse = ListResponse<OAuthClient>;

/** Request body for creating an OAuth client. */
export interface CreateOAuthClientRequest {
  appId: string;
  name: string;
  redirectUris: string[];
  scopes: string[];
  clientType?: "confidential" | "public";
}

/** Response body when an OAuth client is created. */
export interface CreateOAuthClientResponse {
  clientSecret?: string;
  client: OAuthClient;
}

/** Request body for updating an OAuth client. */
export interface UpdateOAuthClientRequest {
  name: string;
  redirectUris: string[];
  scopes: string[];
}

/** Response body when an OAuth client secret is rotated. */
export interface RotateOAuthClientSecretResponse {
  clientSecret: string;
}

/** RFC 6749 §5.1 OAuth token response from the token endpoint. */
export interface OAuthTokenResponse {
  access_token: string;
  token_type: string;
  expires_in: number;
  refresh_token?: string;
  scope: string;
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
  /**
   * Per-request post-callback redirect target.
   * The server enforces `AYB_AUTH_OAUTH_RETURN_TO_ALLOWLIST` host-allowlist
   * validation at both OAuth start AND callback dispatch (see
   * `internal/auth/handler_oauth.go`'s `validatedOAuthReturnTo`). The SDK
   * passes this value through as an opaque string — it does NOT validate
   * the value client-side because the server is the single security owner.
   * If empty or unset, the server falls back to `AYB_AUTH_OAUTH_REDIRECT_URL`.
   * Most useful with the redirect-based flow (`urlCallback`); the popup
   * flow ignores any final redirect because the popup window closes before
   * navigation completes.
   */
  redirectTo?: string;
}

/** Auth state change events emitted by `onAuthStateChange`. */
export type AuthStateEvent = "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED";

/** Auth session payload emitted by `onAuthStateChange`. */
export interface AuthSession {
  token: string;
  refreshToken: string;
}

/** Callback for auth state change events. */
export type AuthStateListener = (
  event: AuthStateEvent,
  session: AuthSession | null,
) => void;

// Collection search settings

/** Searchable text column and ranking weight used by collection search. */
export interface SearchableAttribute {
  column: string;
  weight: "high" | "medium" | "low" | "lowest";
}

/** Deterministic tie-breaker ranking used after text relevance. */
export interface CustomRankingTie {
  column: string;
  order: "asc" | "desc";
}

/** Collection search settings returned by the admin search-settings endpoint. */
export interface SearchSettings {
  attributes: SearchableAttribute[];
  customRanking?: CustomRankingTie[];
}

/** A single equivalence group of terms expanded against each other at query time. */
export interface SearchSynonymGroup {
  terms: string[];
}

/** Request body for PUT /api/collections/{table}/synonyms/. */
export interface SearchSynonymsRequest {
  groups: SearchSynonymGroup[];
}

/** Response body for GET and PUT /api/collections/{table}/synonyms/. */
export interface SearchSynonymsResponse {
  groups: SearchSynonymGroup[];
}
