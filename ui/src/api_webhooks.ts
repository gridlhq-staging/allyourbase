import type {
  WebhookResponse,
  WebhookRequest,
  WebhookTestResult,
  DeliveryListResponse,
  WebhookDelivery,
} from "./types";
import {
  request,
  requestNoBody,
} from "./api_client";

export async function listWebhooks(): Promise<WebhookResponse[]> {
  const res = await request<{ items: WebhookResponse[] }>("/api/webhooks");
  return res.items;
}

export async function createWebhook(
  data: WebhookRequest,
): Promise<WebhookResponse> {
  return request("/api/webhooks", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function updateWebhook(
  id: string,
  data: Partial<WebhookRequest>,
): Promise<WebhookResponse> {
  return request(`/api/webhooks/${id}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function testWebhook(id: string): Promise<WebhookTestResult> {
  return request(`/api/webhooks/${id}/test`, { method: "POST" });
}

export async function listWebhookDeliveries(
  webhookId: string,
  params: { page?: number; perPage?: number; failedOnly?: boolean } = {},
): Promise<DeliveryListResponse> {
  const qs = new URLSearchParams();
  if (params.page) qs.set("page", String(params.page));
  if (params.perPage) qs.set("perPage", String(params.perPage));
  if (params.failedOnly) qs.set("failed_only", "true");
  const suffix = qs.toString() ? `?${qs}` : "";
  return request(`/api/webhooks/${webhookId}/deliveries${suffix}`);
}

export async function replayDelivery(
  webhookId: string,
  deliveryId: string,
): Promise<WebhookDelivery> {
  return request(`/api/webhooks/${webhookId}/deliveries/${deliveryId}/replay`, {
    method: "POST",
  });
}

export async function deleteWebhook(id: string): Promise<void> {
  return requestNoBody(`/api/webhooks/${id}`, {
    method: "DELETE",
  });
}
