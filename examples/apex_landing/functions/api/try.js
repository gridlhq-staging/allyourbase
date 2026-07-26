import { createTryLaunch, TryServiceError } from "../_lib/try_service.js";

function json(payload, status = 200) {
  return Response.json(payload, { status, headers: { "cache-control": "no-store" } });
}

export async function onRequestPost(context) {
  try {
    if (!(context.request.headers.get("content-type") ?? "").includes("form-data")) {
      return json({ error: "Please complete the human check.", code: "challenge_required" }, 400);
    }
    const form = await context.request.formData();
    const result = await createTryLaunch({
      env: context.env,
      clientIp: context.request.headers.get("CF-Connecting-IP") ?? "unknown",
      turnstileToken: String(form.get("cf-turnstile-response") ?? ""),
    });
    return json(result, 201);
  } catch (error) {
    if (error instanceof TryServiceError) return json({ error: error.message, code: error.code }, error.status);
    console.error("try launch failed", error);
    return json({ error: "The launch failed safely. Please try again." }, 500);
  }
}

export function onRequest() {
  return json({ error: "Method not allowed." }, 405);
}
