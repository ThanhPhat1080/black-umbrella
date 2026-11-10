import { DurableObject } from "cloudflare:workers";
import type { Sql } from "./sql";
import { migrate } from "./schema";
import { Auth } from "./auth";
import { Core, type Outbox } from "./core";
import { handleHttp, json } from "./http";

export interface Env {
  HUB: DurableObjectNamespace<HubDO>;
  ADMIN_TOKEN: string;
}

const SWEEP_EVERY_MS = 5 * 60_000;

export class HubDO extends DurableObject<Env> {
  private auth: Auth;
  private core: Core;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    const sql: Sql = {
      exec: <T>(q: string, ...p: unknown[]) => ctx.storage.sql.exec(q, ...(p as never[])).toArray() as T[],
    };
    ctx.blockConcurrencyWhile(async () => migrate(sql));
    const out: Outbox = {
      send: (username, msg) => {
        const data = JSON.stringify(msg);
        for (const ws of ctx.getWebSockets(username)) {
          try {
            ws.send(data);
          } catch {
            /* socket is closing */
          }
        }
      },
      online: () => {
        const names = new Set<string>();
        for (const ws of ctx.getWebSockets()) {
          if (ws.readyState !== WebSocket.OPEN) continue;
          const u = ctx.getTags(ws)[0];
          if (u) names.add(u);
        }
        return [...names];
      },
    };
    this.auth = new Auth(sql, Date.now);
    this.core = new Core(sql, this.auth, out, Date.now);
  }

  private async ensureAlarm(): Promise<void> {
    if ((await this.ctx.storage.getAlarm()) === null) await this.ctx.storage.setAlarm(Date.now() + SWEEP_EVERY_MS);
  }

  async fetch(req: Request): Promise<Response> {
    await this.ensureAlarm();
    const url = new URL(req.url);
    if (url.pathname === "/api/ws") return this.upgrade(req, url);
    const res = await handleHttp(req, this.auth, this.env.ADMIN_TOKEN ?? "", (u) => this.core.userJoined(u.username));
    return res ?? json(404, { error: "not_found" });
  }

  private upgrade(req: Request, url: URL): Response {
    const username = this.auth.userForToken(url.searchParams.get("token") ?? "");
    if (!username) return json(401, { error: "unauthorized" });
    if (req.headers.get("upgrade")?.toLowerCase() !== "websocket") return json(426, { error: "upgrade_required" });
    const pair = new WebSocketPair();
    const [client, server] = [pair[0], pair[1]];
    this.ctx.acceptWebSocket(server, [username]);
    const first = this.ctx.getWebSockets(username).length === 1;
    server.send(JSON.stringify(this.core.hello(username)));
    this.core.onConnect(username, first);
    return new Response(null, { status: 101, webSocket: client });
  }

  async webSocketMessage(ws: WebSocket, message: string | ArrayBuffer): Promise<void> {
    const username = this.ctx.getTags(ws)[0];
    if (username && typeof message === "string") this.core.onMessage(username, message);
  }

  private gone(ws: WebSocket): void {
    const username = this.ctx.getTags(ws)[0];
    if (!username) return;
    const others = this.ctx.getWebSockets(username).filter((s) => s !== ws && s.readyState === WebSocket.OPEN);
    this.core.onDisconnect(username, others.length === 0);
  }

  async webSocketClose(ws: WebSocket, code: number): Promise<void> {
    this.gone(ws);
    try {
      ws.close(code === 1005 || code === 1006 ? 1000 : code);
    } catch {
      /* already closed */
    }
  }

  async webSocketError(ws: WebSocket): Promise<void> {
    this.gone(ws);
  }

  async alarm(): Promise<void> {
    this.core.sweep();
    await this.ctx.storage.setAlarm(Date.now() + SWEEP_EVERY_MS);
  }
}
