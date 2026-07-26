import { getTryLaunchStatus, TryServiceError } from "../../_lib/try_service.js";

function json(payload, status = 200) {
  return Response.json(payload, { status, headers: { "cache-control": "no-store" } });
}

export async function onRequestGet(context) {
  try {
    const launchToken = new URL(context.request.url).searchParams.get("launch") ?? "";
    if (!/^[a-zA-Z0-9-]{8,80}$/.test(launchToken)) {
      return json({ error: "This launch link is invalid." }, 400);
    }
    return json(await getTryLaunchStatus({ env: context.env, launchToken }));
  } catch (error) {
    if (error instanceof TryServiceError) return json({ error: error.message, code: error.code }, error.status);
    console.error("try launch status failed", error);
    return json({ error: "The launch status is temporarily unavailable." }, 500);
  }
}

export function onRequest() {
  return json({ error: "Method not allowed." }, 405);
}
