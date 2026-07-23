/** @module Browser-test fixtures for storage buckets, file uploads, and webhooks. */
import type { APIRequestContext } from "@playwright/test";
import { validateResponse } from "./core";

/** Creates a webhook for the given URL and returns its id and URL. */
export async function seedWebhook(
  request: APIRequestContext,
  token: string,
  url: string,
): Promise<{ id: string; url: string }> {
  const res = await request.post("/api/webhooks", {
    headers: { Authorization: `Bearer ${token}` },
    data: { url, events: ["create"], enabled: true },
  });
  await validateResponse(res, `Create webhook for ${url}`);
  const body = await res.json();
  if (!body.id) {
    throw new Error("Webhook created but no ID in response");
  }
  return { id: String(body.id), url: body.url };
}

export async function deleteWebhook(
  request: APIRequestContext,
  token: string,
  id: string,
): Promise<void> {
  const res = await request.delete(`/api/webhooks/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Delete webhook ${id}`);
}

/** Lists all webhooks and deletes those whose URL is in the given set. */
export async function deleteWebhooksByURL(
  request: APIRequestContext,
  token: string,
  urls: string[],
): Promise<void> {
  if (urls.length === 0) {
    return;
  }
  const res = await request.get("/api/webhooks", {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, "List webhooks for URL cleanup");
  const body = await res.json();
  const items = Array.isArray(body?.items) ? body.items : [];
  const urlSet = new Set(urls);
  for (const item of items) {
    if (typeof item?.url !== "string" || !urlSet.has(item.url)) {
      continue;
    }
    if (typeof item?.id === "string" || typeof item?.id === "number") {
      await deleteWebhook(request, token, String(item.id)).catch(() => {});
    }
  }
}

/** Uploads a file via multipart POST to a storage bucket and returns its name. */
export async function seedFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
  content: string,
): Promise<{ name: string }> {
  const res = await request.post(`/api/storage/${encodeURIComponent(bucket)}`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      file: {
        name: fileName,
        mimeType: "text/plain",
        buffer: Buffer.from(content),
      },
    },
  });
  await validateResponse(res, `Upload file ${fileName} to bucket ${bucket}`);
  const body = await res.json();
  if (!body.name) {
    throw new Error("File upload succeeded but no name in response");
  }
  return { name: body.name };
}

/** Creates a storage bucket, or updates it on 409 conflict if it already exists. */
export async function ensureStorageBucket(
  request: APIRequestContext,
  token: string,
  bucket: string,
  publicBucket = true,
): Promise<void> {
  const createRes = await request.post("/api/storage/buckets/", {
    headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
    data: { name: bucket, public: publicBucket },
  });
  if (createRes.status() === 409) {
    const updateRes = await request.put(`/api/storage/buckets/${encodeURIComponent(bucket)}`, {
      headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
      data: { public: publicBucket },
    });
    await validateResponse(updateRes, `Update storage bucket ${bucket}`);
    return;
  }
  await validateResponse(createRes, `Create storage bucket ${bucket}`);
}

export async function deleteStorageBucket(
  request: APIRequestContext,
  token: string,
  bucket: string,
  force = true,
): Promise<void> {
  const res = await request.delete(
    `/api/storage/buckets/${encodeURIComponent(bucket)}${force ? "?force=true" : ""}`,
    { headers: { Authorization: `Bearer ${token}` } },
  );
  await validateResponse(res, `Delete storage bucket ${bucket}`);
}

export async function deleteFile(
  request: APIRequestContext,
  token: string,
  bucket: string,
  fileName: string,
): Promise<void> {
  const res = await request.delete(
    `/api/storage/${encodeURIComponent(bucket)}/${encodeURIComponent(fileName)}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  await validateResponse(res, `Delete file ${fileName} from bucket ${bucket}`);
}

export async function seedRecord(
  request: APIRequestContext,
  token: string,
  table: string,
  data: Record<string, unknown>,
): Promise<Record<string, unknown>> {
  const res = await request.post(`/api/collections/${encodeURIComponent(table)}`, {
    headers: { Authorization: `Bearer ${token}` },
    data,
  });
  await validateResponse(res, `Create record in table ${table}`);
  return await res.json();
}

export async function listRecords(
  request: APIRequestContext,
  token: string,
  table: string,
): Promise<Record<string, unknown>[]> {
  const res = await request.get(`/api/collections/${encodeURIComponent(table)}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List records in table ${table}`);
  const body = await res.json();
  if (!body || !Array.isArray(body.items)) {
    throw new Error(`List records in table ${table} returned invalid response shape`);
  }
  return body.items as Record<string, unknown>[];
}
