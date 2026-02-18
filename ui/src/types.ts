// Schema types matching Go's schema.SchemaCache JSON output.

export interface SchemaCache {
  tables: Record<string, Table>;
  functions?: Record<string, SchemaFunction>;
  schemas: string[];
  builtAt: string;
}

export interface Table {
  schema: string;
  name: string;
  kind: string;
  comment?: string;
  columns: Column[];
  primaryKey: string[];
  foreignKeys?: ForeignKey[];
  indexes?: Index[];
  relationships?: Relationship[];
}

export interface Column {
  name: string;
  position: number;
  type: string;
  nullable: boolean;
  default?: string;
  comment?: string;
  isPrimaryKey: boolean;
  jsonType: string;
  enumValues?: string[];
}

export interface ForeignKey {
  constraintName: string;
  columns: string[];
  referencedSchema: string;
  referencedTable: string;
  referencedColumns: string[];
  onUpdate?: string;
  onDelete?: string;
}

export interface Index {
  name: string;
  isUnique: boolean;
  isPrimary: boolean;
  method: string;
  definition: string;
}

export interface Relationship {
  name: string;
  type: string;
  fromSchema: string;
  fromTable: string;
  fromColumns: string[];
  toSchema: string;
  toTable: string;
  toColumns: string[];
  fieldName: string;
}

// SQL editor response.
export interface SqlResult {
  columns: string[];
  rows: unknown[][];
  rowCount: number;
  durationMs: number;
}

// API list response envelope.
export interface ListResponse {
  items: Record<string, unknown>[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// Webhook CRUD types (matches Go webhookResponse).
export interface WebhookResponse {
  id: string;
  url: string;
  hasSecret: boolean;
  events: string[];
  tables: string[];
  enabled: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface WebhookRequest {
  url: string;
  secret?: string;
  events?: string[];
  tables?: string[];
  enabled?: boolean;
}

export interface WebhookTestResult {
  success: boolean;
  statusCode?: number;
  durationMs: number;
  error?: string;
}

// Webhook delivery log types.
export interface WebhookDelivery {
  id: string;
  webhookId: string;
  eventAction: string;
  eventTable: string;
  success: boolean;
  statusCode: number;
  attempt: number;
  durationMs: number;
  error?: string;
  requestBody?: string;
  responseBody?: string;
  deliveredAt: string;
}

export interface DeliveryListResponse {
  items: WebhookDelivery[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// Admin user management types.
export interface AdminUser {
  id: string;
  email: string;
  emailVerified: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface UserListResponse {
  items: AdminUser[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

// RPC function types (matches Go schema.Function).
export interface FuncParam {
  name: string;
  type: string;
  position: number;
}

export interface SchemaFunction {
  schema: string;
  name: string;
  comment?: string;
  parameters: FuncParam[] | null;
  returnType: string;
  returnsSet: boolean;
  isVoid: boolean;
}

// API key types.
export interface APIKeyResponse {
  id: string;
  userId: string;
  name: string;
  keyPrefix: string;
  scope: string;
  allowedTables: string[] | null;
  lastUsedAt: string | null;
  expiresAt: string | null;
  createdAt: string;
  revokedAt: string | null;
}

export interface APIKeyListResponse {
  items: APIKeyResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface APIKeyCreateResponse {
  key: string;
  apiKey: APIKeyResponse;
}

// API Explorer types.
export interface ApiExplorerRequest {
  method: string;
  path: string;
  body?: string;
}

export interface ApiExplorerResponse {
  status: number;
  statusText: string;
  headers: Record<string, string>;
  body: string;
  durationMs: number;
}

export interface ApiExplorerHistoryEntry {
  method: string;
  path: string;
  body?: string;
  status: number;
  durationMs: number;
  timestamp: string;
}

// RLS policy types.
export interface RlsPolicy {
  tableSchema: string;
  tableName: string;
  policyName: string;
  command: string;
  permissive: string;
  roles: string[];
  usingExpr: string | null;
  withCheckExpr: string | null;
}

export interface RlsTableStatus {
  rlsEnabled: boolean;
  forceRls: boolean;
}

// Storage types.
export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}

export interface StorageListResponse {
  items: StorageObject[];
  totalItems: number;
}
