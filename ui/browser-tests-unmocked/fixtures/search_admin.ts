import type { APIRequestContext } from "@playwright/test";
import type { CollectionSearchSettingsPayload } from "../../src/api_admin";
import { validateResponse } from "./core";

const SEARCH_ADMIN_RETRY_DELAYS_MS = [100, 200, 400, 800, 1200];

interface CollectionSearchSynonymGroup {
  terms: string[];
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function isTransientSearchAdminError(status: number, message: string): boolean {
  if (status === 503 && message.includes("schema cache not ready")) {
    return true;
  }
  if (status === 404 && message.includes("collection not found")) {
    return true;
  }
  return status === 500 && message.includes("failed to refresh search indexes");
}

async function describeResponseError(
  res: Awaited<ReturnType<APIRequestContext["put"]>>,
  context: string,
): Promise<string> {
  const status = res.status();
  let message = `${context} failed with status ${status}`;
  try {
    const body = await res.json();
    if (body.message) {
      message += `: ${body.message}`;
    }
    if (body.code) {
      message += ` (code: ${body.code})`;
    }
  } catch {
    const text = await res.text();
    if (text) {
      message += `: ${text}`;
    }
  }
  return message;
}

async function putSearchAdminResource(
  request: APIRequestContext,
  token: string,
  path: string,
  data: unknown,
  context: string,
): Promise<Awaited<ReturnType<APIRequestContext["put"]>>> {
  let lastError = "";
  for (let attempt = 0; attempt <= SEARCH_ADMIN_RETRY_DELAYS_MS.length; attempt += 1) {
    const res = await request.put(path, {
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      data,
    });
    if (res.ok()) {
      return res;
    }
    const errorMessage = await describeResponseError(res, context);
    lastError = errorMessage;
    if (!isTransientSearchAdminError(res.status(), errorMessage)) {
      throw new Error(errorMessage);
    }
    const delay = SEARCH_ADMIN_RETRY_DELAYS_MS[attempt];
    if (delay === undefined) {
      break;
    }
    await sleep(delay);
  }
  throw new Error(lastError || `${context} failed before receiving a response`);
}

/**
 * Seeds or replaces the full synonym-group set for a collection through the
 * shipped admin contract instead of writing the backing table directly.
 */
export async function replaceCollectionSearchSynonyms(
  request: APIRequestContext,
  token: string,
  tableName: string,
  groups: CollectionSearchSynonymGroup[],
): Promise<{ groups: CollectionSearchSynonymGroup[] }> {
  const res = await putSearchAdminResource(
    request,
    token,
    `/api/collections/${encodeURIComponent(tableName)}/synonyms`,
    { groups },
    `Replace search synonyms for ${tableName}`,
  );
  await validateResponse(res, `Replace search synonyms for ${tableName}`);
  const body = await res.json();
  if (!Array.isArray(body?.groups)) {
    throw new Error(`Expected synonym groups array for collection ${tableName}`);
  }
  return body as { groups: CollectionSearchSynonymGroup[] };
}

/**
 * Seeds or replaces the full search-settings payload for a collection through
 * the shipped admin contract instead of writing the backing table directly.
 */
export async function replaceCollectionSearchSettings(
  request: APIRequestContext,
  token: string,
  tableName: string,
  payload: CollectionSearchSettingsPayload,
): Promise<CollectionSearchSettingsPayload> {
  const res = await putSearchAdminResource(
    request,
    token,
    `/api/collections/${encodeURIComponent(tableName)}/search-settings`,
    payload,
    `Replace search settings for ${tableName}`,
  );
  await validateResponse(res, `Replace search settings for ${tableName}`);
  const body = await res.json();
  if (!Array.isArray(body?.attributes) || !Array.isArray(body?.customRanking)) {
    throw new Error(`Expected search settings arrays for collection ${tableName}`);
  }
  return body as CollectionSearchSettingsPayload;
}
