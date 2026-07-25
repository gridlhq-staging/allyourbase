/**
 * @module Browser-test fixtures for infrastructure resource seeding (sites, domains, replicas, backups, FDW).
 */
import type { APIRequestContext } from "@playwright/test";
import { execSQL, sqlLiteral, validateResponse } from "./core";

/** Creates a hosting site via the admin API and returns its id, name, and slug. */
export async function seedSite(
  request: APIRequestContext,
  token: string,
  options: { name?: string; slug?: string } = {},
): Promise<{ id: string; name: string; slug: string }> {
  const runID = Date.now();
  const name = options.name || `smoke-site-${runID}`;
  const slug = options.slug || `smoke-site-${runID}`;
  const res = await request.post("/api/admin/sites", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name, slug },
  });
  await validateResponse(res, `Create site ${name}`);
  const body = await res.json();
  if (
    typeof body?.id !== "string" ||
    typeof body?.name !== "string" ||
    typeof body?.slug !== "string"
  ) {
    throw new Error(`Expected seeded site fields id/name/slug for ${name}`);
  }
  return { id: body.id, name: body.name, slug: body.slug };
}

export async function cleanupSiteByID(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete site ${siteID}`);
  }
}

/** Fetches a site by ID and returns its fields, normalizing both camelCase and snake_case spaMode variants. */
export async function getSite(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<{ id: string; name: string; slug: string; spaMode: boolean }> {
  const res = await request.get(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Get site ${siteID}`);
  const body = await res.json();

  const spaModeValue = body?.spaMode ?? body?.spa_mode;
  if (
    typeof body?.id !== "string" ||
    typeof body?.name !== "string" ||
    typeof body?.slug !== "string" ||
    typeof spaModeValue !== "boolean"
  ) {
    throw new Error(`Expected site payload id/name/slug/spaMode for ${siteID}`);
  }

  return {
    id: body.id,
    name: body.name,
    slug: body.slug,
    spaMode: spaModeValue,
  };
}

export async function getSiteStatus(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<number> {
  const res = await request.get(`/api/admin/sites/${encodeURIComponent(siteID)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return res.status();
}

/** Creates a custom domain with a hostname and environment, returning id, hostname, status, and verification record. */
export async function seedCustomDomain(
  request: APIRequestContext,
  token: string,
  hostname: string,
  options: { environment?: string } = {},
): Promise<{
  id: string;
  hostname: string;
  status: string;
  environment: string;
  verificationRecord: string;
}> {
  const res = await request.post("/api/admin/domains", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { hostname, environment: options.environment || "staging" },
  });
  await validateResponse(res, `Create custom domain ${hostname}`);
  const body = await res.json();
  if (
    typeof body?.id !== "string" ||
    typeof body?.hostname !== "string" ||
    typeof body?.status !== "string" ||
    typeof body?.environment !== "string" ||
    typeof body?.verificationRecord !== "string"
  ) {
    throw new Error(`Expected seeded custom domain fields for ${hostname}`);
  }
  return {
    id: body.id,
    hostname: body.hostname,
    status: body.status,
    environment: body.environment,
    verificationRecord: body.verificationRecord,
  };
}

export async function cleanupCustomDomain(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/domains/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete custom domain ${id}`);
  }
}

/** Creates a log drain via the admin API and returns its id and name. */
export async function seedLogDrain(
  request: APIRequestContext,
  token: string,
  options: {
    name?: string;
    type?: string;
    url?: string;
    batch_size?: number;
    flush_interval_sec?: number;
  },
): Promise<{ id: string; name: string }> {
  const name = options.name || `test-drain-${Date.now()}`;
  const res = await request.post("/api/admin/logging/drains", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      id: name,
      type: options.type || "http",
      url: options.url || "https://example.com/logs",
      batch_size: options.batch_size || 100,
      flush_interval_seconds: options.flush_interval_sec || 5,
    },
  });
  await validateResponse(res, `Create log drain ${name}`);
  const body = await res.json();
  if (typeof body?.id !== "string" || typeof body?.name !== "string") {
    throw new Error(`Expected seeded log drain fields for ${name}`);
  }
  return { id: body.id, name: body.name };
}

export async function cleanupLogDrain(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/logging/drains/${encodeURIComponent(id)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete log drain ${id}`);
  }
}

const testSAMLSigningCertificate = [
  "MIIDJzCCAg+gAwIBAgIUdF2pLOFwCZ0wgaHe9ZKu8MN6tZ8wDQYJKoZIhvcNAQELBQAwIzEh",
  "MB8GA1UEAwwYQVlCIHNoYXJlZCB0ZXN0IFNBTUwgSWRQMB4XDTI2MDcyNTAyNDUzNFoXDTM2",
  "MDcyMjAyNDUzNFowIzEhMB8GA1UEAwwYQVlCIHNoYXJlZCB0ZXN0IFNBTUwgSWRQMIIBIjAN",
  "BgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAqf+jAc+fabuM61lmIL+WgqWHYR7wCFNJULB",
  "J/48qWN/rx3SSWpgXdByyP7ur20PMK+Sm8yiZvp2QGUCCv/3jzB1R/AH1S6R3PPfy9fabDT",
  "EuWrWz/HNXZ+GzY36UeB+TAcKZycJgptCvZKkEncuQcgu2d7G3uFE6NB6KDbASNhM9Fci54k",
  "QshdhUOe0D28p+Pjni4OBEgngf2BOln/xBUl5K1djcSUHP9QygNn95ESy4deynHwnejmO47",
  "MLktr7oIi1LGv+nNFSdIxIfZCuuDStgZEJEz1TL8tinOhxQ/o936vXQ/ied6hTuMECrz+cOI",
  "4T2G9RfiCrf0rFsb+EOzwIDAQABo1MwUTAdBgNVHQ4EFgQURkeI2vHjEjkTN54WT0EB5y7i",
  "slkwHwYDVR0jBBgwFoAURkeI2vHjEjkTN54WT0EB5y7islkwDwYDVR0TAQH/BAUwAwEB/zAN",
  "BgkqhkiG9w0BAQsFAAOCAQEAUPxeVZTPA8yXPENGR+41YyWXzR2VzaXPU0C+rtrXw2T4La0E",
  "7JhGUl/iEIY3U4xyC9fwb/W7hh3MjeaSayisN62EYfCf4aPLq6YROwFJco2d5e8E7D83RFE8",
  "EApwLbYhl8xT/CUUmz5yRQaYcnwtNIiiwkeNTjl+WoFsEHHWFML3EWMdme2eVOd8J06SfA0i",
  "BbmoXgr5TZKWr73Juo4pPFGe3DVIjbXfKm01dz9rMXgllHgU5xgRg0RLk7M/aUUNWYnM2jUE",
  "NCFhsxbpbVezIlJY/rASMifuY9ktnB4hfEjR1+G3AfaM0tBahhK5rK1Oz74GkfH9V1twim5a",
  "cnSGjQ==",
].join("");

export function buildTestSAMLMetadataXML(options: { name: string; entity_id: string }): string {
  return `<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="${options.entity_id}">
  <IDPSSODescriptor>
    <KeyDescriptor use="signing">
      <KeyInfo xmlns="http://www.w3.org/2000/09/xmldsig#">
        <X509Data><X509Certificate>${testSAMLSigningCertificate}</X509Certificate></X509Data>
      </KeyInfo>
    </KeyDescriptor>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.test/${options.name}/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`;
}

/** Creates a SAML provider with generated XML metadata and returns its name and entity_id. */
export async function seedSAMLProvider(
  request: APIRequestContext,
  token: string,
  options: {
    name?: string;
    entity_id?: string;
    idp_metadata_xml?: string;
    idp_metadata_url?: string;
    metadata_url?: string;
  },
): Promise<{ name: string; entity_id: string }> {
  const name = options.name || `test-saml-${Date.now()}`;
  const entityId = options.entity_id || `urn:test:${name}`;
  const idpMetadataXML =
    options.idp_metadata_xml || buildTestSAMLMetadataXML({ name, entity_id: entityId });
  const idpMetadataURL = options.idp_metadata_url || options.metadata_url;
  const res = await request.post("/api/admin/auth/saml", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name,
      entity_id: entityId,
      idp_metadata_xml: idpMetadataXML,
      ...(idpMetadataURL ? { idp_metadata_url: idpMetadataURL } : {}),
    },
  });
  await validateResponse(res, `Create SAML provider ${name}`);
  return { name, entity_id: entityId };
}

export async function cleanupSAMLProvider(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/auth/saml/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete SAML provider ${name}`);
  }
}

export async function seedSecret(
  request: APIRequestContext,
  token: string,
  name: string,
  value: string,
): Promise<void> {
  const res = await request.post("/api/admin/secrets", {
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    data: { name, value },
  });
  await validateResponse(res, `Create secret ${name}`);
}

export async function cleanupSecret(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/secrets/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete secret ${name}`);
  }
}

export async function seedBranch(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<{ name: string }> {
  const res = await request.post("/api/admin/branches/", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name },
  });
  await validateResponse(res, `Create branch ${name}`);
  return { name };
}

export async function cleanupBranch(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/branches/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Delete branch ${name}`);
  }
}

export type ReplicaSeedTarget = {
  host: string;
  port: number;
  database: string;
  sslMode: string;
};

/**
 * Resolves a reachable standby from AYB_DATABASE_REPLICA_URLS for replica seeding.
 * Returns null when no valid target is configured, which callers treat as a skip
 * precondition. This is the single parser for that variable across browser specs.
 */
export function resolveReplicaSeedTarget(): ReplicaSeedTarget | null {
  const replicaURL = process.env.AYB_DATABASE_REPLICA_URLS
    ?.split(",")
    .map((value) => value.trim())
    .find((value) => value.length > 0);
  if (!replicaURL) {
    return null;
  }

  try {
    const parsed = new URL(replicaURL);
    const database = parsed.pathname.replace(/^\/+/, "");
    return {
      host: parsed.hostname,
      port: parsed.port ? Number(parsed.port) : 5432,
      database: database || "postgres",
      sslMode: parsed.searchParams.get("sslmode") || "disable",
    };
  } catch {
    return null;
  }
}

/** Creates a read replica with the given connection config and returns its name. */
export async function seedReplica(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    host: string;
    port?: number;
    database: string;
    ssl_mode?: string;
    weight?: number;
    max_lag_bytes?: number;
  },
): Promise<{ name: string }> {
  const res = await request.post("/api/admin/replicas", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: options.name,
      host: options.host,
      port: options.port ?? 5432,
      database: options.database,
      ssl_mode: options.ssl_mode || "disable",
      weight: options.weight ?? 100,
      max_lag_bytes: options.max_lag_bytes ?? 0,
    },
  });
  await validateResponse(res, `Add replica ${options.name}`);
  return { name: options.name };
}

/** Fetches all replicas and returns an array of {url, state} status entries. */
export async function fetchReplicaStatuses(
  request: APIRequestContext,
  token: string,
): Promise<{ replicas: Array<{ url: string; state: string }> }> {
  const res = await request.get("/api/admin/replicas", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List replicas");
  const body = await res.json();
  if (!Array.isArray(body?.replicas)) {
    throw new Error("Expected replicas array from /api/admin/replicas");
  }
  return {
    replicas: body.replicas.map((replica: unknown) => {
      if (
        !replica ||
        typeof replica !== "object" ||
        typeof (replica as { url?: unknown }).url !== "string" ||
        typeof (replica as { state?: unknown }).state !== "string"
      ) {
        throw new Error("Expected replica status entries with url/state fields");
      }
      return {
        url: (replica as { url: string }).url,
        state: (replica as { state: string }).state,
      };
    }),
  };
}

export async function cleanupReplicaByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/replicas/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Remove replica ${name}`);
  }
}

export async function enableExtension(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.post("/api/admin/extensions", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name },
  });
  await validateResponse(res, `Enable extension ${name}`);
}

export async function disableExtension(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(`/api/admin/extensions/${encodeURIComponent(name)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (res.status() !== 404) {
    await validateResponse(res, `Disable extension ${name}`);
  }
}

/** Inserts a backup row via SQL (no domain API) and returns its id and dbName. */
export async function seedBackup(
  request: APIRequestContext,
  token: string,
  options: {
    dbName: string;
    status?: string;
    backupType?: string;
    triggeredBy?: string;
    sizeBytes?: number;
  },
): Promise<{ id: string; dbName: string }> {
  const dbName = sqlLiteral(options.dbName);
  const status = sqlLiteral(options.status || "completed");
  const backupType = sqlLiteral(options.backupType || "logical");
  const triggeredBy = sqlLiteral(options.triggeredBy || "smoke-test");
  const sizeBytes = options.sizeBytes ?? 1048576;
  if (!Number.isSafeInteger(sizeBytes) || sizeBytes < 0) {
    throw new Error("sizeBytes must be a non-negative safe integer");
  }
  // Stage 1 product gap: backup metadata has no deterministic seed API.
  const result = await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: backup metadata has no deterministic seed API.
    `INSERT INTO _ayb_backups (db_name, status, backup_type, triggered_by, size_bytes, started_at, completed_at)
     VALUES ('${dbName}', '${status}', '${backupType}', '${triggeredBy}', ${sizeBytes}, NOW() - INTERVAL '5 minutes', NOW())
     RETURNING id`,
  );
  const id = result.rows[0]?.[0];
  if (typeof id !== "string") {
    throw new Error(`Expected backup id for db_name ${options.dbName}`);
  }
  return { id, dbName: options.dbName };
}

export async function cleanupBackupsByDbName(
  request: APIRequestContext,
  token: string,
  dbName: string,
): Promise<void> {
  // Stage 1 product gap: backup metadata has no domain cleanup API.
  await execSQL(
    request,
    token,
    // eslint-disable-next-line no-restricted-syntax -- Stage 1 product gap: backup metadata has no domain cleanup API.
    `DELETE FROM _ayb_backups WHERE db_name = '${sqlLiteral(dbName)}'`,
  );
}

/** Creates an FDW server (postgres_fdw or file_fdw) via the admin API and returns its name and type. */
export async function seedFDWServer(
  request: APIRequestContext,
  token: string,
  options: {
    name: string;
    fdwType?: string;
    host?: string;
    port?: number;
    dbname?: string;
    filename?: string;
    user?: string;
    password?: string;
  },
): Promise<{ name: string; fdwType: string }> {
  const fdwType = options.fdwType || "postgres_fdw";
  const serverOptions: Record<string, string> =
    fdwType === "postgres_fdw"
      ? {
          host: options.host || "localhost",
          port: String(options.port ?? 5432),
          dbname: options.dbname || "postgres",
        }
      : { filename: options.filename || "/dev/null" };
  const res = await request.post("/api/admin/fdw/servers", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: {
      name: options.name,
      fdw_type: fdwType,
      options: serverOptions,
      ...(fdwType === "postgres_fdw"
        ? {
            user_mapping: {
              user: options.user || "postgres",
              password: options.password || "postgres",
            },
          }
        : {}),
    },
  });
  await validateResponse(res, `Create FDW server ${options.name}`);
  return { name: options.name, fdwType };
}

export async function cleanupFDWServer(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const res = await request.delete(
    `/api/admin/fdw/servers/${encodeURIComponent(name)}?cascade=true`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  if (res.status() !== 404) {
    await validateResponse(res, `Drop FDW server ${name}`);
  }
}

export async function seedAIPrompt(
  request: APIRequestContext,
  token: string,
  options: { name: string; template: string },
): Promise<{ id: string; name: string }> {
  const res = await request.post("/api/admin/ai/prompts", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name: options.name, template: options.template },
  });
  await validateResponse(res, `Create AI prompt ${options.name}`);
  const body = await res.json();
  return { id: body.id, name: body.name };
}

/** Lists AI prompts and deletes all that match the given name. */
export async function cleanupAIPromptByName(
  request: APIRequestContext,
  token: string,
  name: string,
): Promise<void> {
  const listRes = await request.get("/api/admin/ai/prompts?perPage=100", {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!listRes.ok()) {
    return;
  }
  const body = await listRes.json();
  const prompts = body.prompts || body.items || [];
  for (const prompt of prompts) {
    if (prompt.name === name && prompt.id) {
      const delRes = await request.delete(`/api/admin/ai/prompts/${encodeURIComponent(prompt.id)}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (delRes.status() !== 404) {
        await validateResponse(delRes, `Delete AI prompt ${prompt.id}`);
      }
    }
  }
}

// Fixture helper: list backups via the admin API.
// Extracted from spec files to comply with eslint no-restricted-syntax rule.
export async function listBackups(
  request: APIRequestContext,
  adminToken: string,
): Promise<{ ok: boolean; backups: Array<{ db_name?: string; [key: string]: unknown }> }> {
  const res = await request.get("/api/admin/backups", {
    headers: { Authorization: `Bearer ${adminToken}` },
  });
  if (!res.ok()) {
    return { ok: false, backups: [] };
  }
  const body = await res.json();
  const backups = Array.isArray(body?.backups) ? body.backups : [];
  return { ok: true, backups };
}
