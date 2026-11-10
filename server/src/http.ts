import { Auth, ApiError, type User } from "./auth";

const JSON_HEADERS = { "content-type": "application/json" };

export function json(status: number, body: unknown): Response {
  return new Response(JSON.stringify(body), { status, headers: JSON_HEADERS });
}

function timingSafeEqual(a: string, b: string): boolean {
  const ea = new TextEncoder().encode(a);
  const eb = new TextEncoder().encode(b);
  let diff = ea.length ^ eb.length;
  const n = Math.max(ea.length, eb.length);
  for (let i = 0; i < n; i++) diff |= (ea[i] ?? 0) ^ (eb[i] ?? 0);
  return diff === 0;
}

async function readBody(req: Request): Promise<Record<string, unknown>> {
  try {
    const b = await req.json();
    return b && typeof b === "object" ? (b as Record<string, unknown>) : {};
  } catch {
    return {};
  }
}

const ROUTES = new Set([
  "/api/register",
  "/api/login",
  "/api/admin/invite",
  "/api/admin/reset",
  "/api/admin/unlock",
]);

/** Returns null when the request is not an auth route. `onRegistered` runs after a successful registration. */
export async function handleHttp(
  req: Request,
  auth: Auth,
  adminToken: string,
  onRegistered?: (user: User) => void,
): Promise<Response | null> {
  const path = new URL(req.url).pathname;
  if (!ROUTES.has(path)) return null;
  if (req.method !== "POST") return json(405, { error: "method_not_allowed" });
  try {
    if (path.startsWith("/api/admin/")) {
      const h = req.headers.get("authorization") ?? "";
      const given = h.startsWith("Bearer ") ? h.slice(7) : "";
      if (!adminToken || !timingSafeEqual(given, adminToken)) throw new ApiError(401, "unauthorized");
      if (path === "/api/admin/invite") return json(200, { code: auth.createInvite() });
      const b = await readBody(req);
      const username = typeof b.username === "string" ? b.username : "";
      if (path === "/api/admin/reset") return json(200, { code: auth.reset(username) });
      auth.unlock(username);
      return json(200, { ok: true });
    }
    const b = await readBody(req);
    if (path === "/api/register") {
      const r = await auth.register(b);
      onRegistered?.(r.user);
      return json(200, r);
    }
    const token = await auth.login(String(b.username ?? ""), String(b.pin ?? ""));
    return json(200, { token });
  } catch (e) {
    if (e instanceof ApiError) return json(e.status, { error: e.code, ...e.extra });
    throw e;
  }
}
