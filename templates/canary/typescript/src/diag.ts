// The diagnostics the platform's canary drives to prove the container's boundaries
// (HARNESS.md H18, H22, H37 and H42). Every route is private, answers JSON, and is safe to delete in
// your own app. They are the same in the three templates.
import { accessSync, constants, existsSync } from "node:fs";
import { request } from "node:http";
import { networkInterfaces } from "node:os";
import postgres from "postgres";

// The edge's internal listener, where app-to-app calls arrive (CADDY.md §4.3).
const internalPort = "8443";

const writable = (p: string) => { try { accessSync(p, constants.W_OK); return true; } catch { return false; } };

// The container's IPv4 addresses in CIDR form, loopback left out.
const ownAddrs = (): string[] =>
  Object.values(networkInterfaces()).flatMap((list) => list ?? [])
    .filter((a) => a.family === "IPv4" && !a.internal)
    .map((a) => a.cidr ?? a.address);

const dbIdentity = (raw: string): { role: string; database: string } => {
  try {
    const u = new URL(raw);
    return { role: decodeURIComponent(u.username), database: u.pathname.replace(/^\//, "") };
  } catch {
    return { role: "", database: "" };
  }
};

// The process user, whether / and /tmp take writes, the container's own addresses, whether the
// host's Docker socket is visible, and the database role and name.
export const diagReport = () => {
  const { role, database } = dbIdentity(process.env.DATABASE_URL ?? "");
  return {
    uid: process.getuid?.() ?? null,
    root_writable: writable("/"),
    tmp_writable: writable("/tmp"),
    addrs: ownAddrs(),
    docker_socket: existsSync("/var/run/docker.sock"),
    db_role: role,
    db_name: database,
    node: process.version,
  };
};

// One HTTP GET with an explicit Host header (fetch forbids setting one).
const getWithHost = (host: string, port: string, path: string, headers: Record<string, string>) =>
  new Promise<{ status: number; body: string }>((resolve, reject) => {
    const req = request({ host, port, path, method: "GET", headers, timeout: 5000 }, (res) => {
      let body = "";
      res.setEncoding("utf8");
      res.on("data", (chunk) => { body += chunk; });
      res.on("end", () => resolve({ status: res.statusCode ?? 0, body }));
    });
    req.on("timeout", () => req.destroy(new Error("timeout")));
    req.on("error", reject);
    req.end();
  });

// One request to the named app's internal name with this app's service identity (or none):
// the status the edge answered and the caller the target app saw. Only the apps this app
// declares in `calls` resolve from inside the container; `via` names one of them, and the
// request is sent to its address with `to` as the host, which is how the edge's refusal of an
// undeclared target is observed.
export const diagCall = async (to: string, withToken: boolean, via?: string) => {
  if (!to) return { error: "to is required: the app id to call" };
  const url = `http://${via || to}.internal.whisk:${internalPort}/whoami`;
  const headers: Record<string, string> = { accept: "application/json", host: `${to}.internal.whisk:${internalPort}` };
  if (withToken) headers.authorization = `Bearer ${process.env.WHISK_SERVICE_TOKEN ?? ""}`;
  try {
    const resp = await getWithHost(`${via || to}.internal.whisk`, internalPort, "/whoami", headers);
    let echoed: Record<string, unknown> = {};
    try { echoed = JSON.parse(resp.body); } catch { echoed = {}; }
    const error = echoed.error as { code?: string } | undefined;
    return { to, url, status: resp.status, service_app: echoed["x-whisk-service-app"] ?? "", audience: echoed["x-whisk-audience"] ?? "", code: error?.code ?? "" };
  } catch (e) {
    return { to, url, status: 0, error: String(e) };
  }
};

// A connection to the app's own database endpoint as another role or to another database,
// which the platform must refuse.
export const diagPG = async (role?: string, database?: string) => {
  let u: URL;
  try { u = new URL(process.env.DATABASE_URL ?? ""); } catch { return { error: "DATABASE_URL is not a URL" }; }
  const asRole = role || decodeURIComponent(u.username);
  const asDatabase = database || u.pathname.replace(/^\//, "");
  u.username = asRole;
  u.pathname = "/" + asDatabase;
  const client = postgres(u.toString(), { max: 1, connect_timeout: 5, prepare: false });
  try {
    await client`select 1`;
    return { role: asRole, database: asDatabase, connected: true, error: "" };
  } catch (e) {
    return { role: asRole, database: asDatabase, connected: false, error: String(e) };
  } finally {
    await client.end({ timeout: 1 }).catch(() => undefined);
  }
};

export type DiagPost = { app_id?: unknown; path?: unknown; headers?: unknown; body?: unknown };

// POST /diag/call with a JSON body {app_id, path, headers, body}: one POST to the named app's
// internal name at path, carrying this app's service token, the given headers (a map of name
// to value, optional) and the body as sent. Answers {status, body, headers} with what came
// back, or {status: 0, error} when the call never completed; null when the input is invalid,
// which the route answers 400. The harness uses it to show that the delivery headers an app
// forges on an app-to-app call are stripped and that a handler refuses the call.
export const diagPost = async (input: DiagPost) => {
  const { app_id: appId, path, headers = {}, body = "" } = input;
  if (typeof appId !== "string" || !appId || typeof path !== "string" || !path.startsWith("/") || typeof body !== "string" || typeof headers !== "object" || headers === null) return null;
  const sent: Record<string, string> = { ...Object.fromEntries(Object.entries(headers).map(([k, v]) => [k, String(v)])), authorization: `Bearer ${process.env.WHISK_SERVICE_TOKEN ?? ""}` };
  try {
    const resp = await fetch(`http://${appId}.internal.whisk:${internalPort}${path}`, { method: "POST", headers: sent, body, signal: AbortSignal.timeout(10000) });
    return { status: resp.status, body: (await resp.text()).slice(0, 1 << 16), headers: Object.fromEntries(resp.headers.entries()) };
  } catch (e) {
    return { status: 0, error: String(e) };
  }
};

// The shape of an app's database role, and so of its schema in a shared database
// (CONTRACT.md §8).
const shareRole = /^app_[a-z0-9_]{1,59}$/;

// One connection to the app's own database, with no prepared statements because DATABASE_URL
// is pooled in transaction mode.
const diagConn = () => postgres(process.env.DATABASE_URL ?? "", { max: 1, connect_timeout: 5, prepare: false });

export type DiagShare = { to?: unknown; marker?: unknown };

// POST /diag/pg/share with a JSON body {to, marker}: in a shared database, the app makes
// diag_share_existing in its own schema and stores the marker, grants the role `to` read access
// exactly as SKILL.md §5 tells an agent to, then makes diag_share_new and stores the marker
// there too. Answers {schema} or {error}; null when the input is invalid.
export const diagShare = async (input: DiagShare) => {
  const { to, marker } = input;
  if (typeof to !== "string" || !shareRole.test(to) || typeof marker !== "string" || marker === "") return null;
  const client = diagConn();
  let schema = "";
  try {
    [{ schema }] = await client<{ schema: string }[]>`select current_user as schema`;
    const s = client(schema);
    const t = client(to);
    await client`create table if not exists ${s}.diag_share_existing (marker text not null)`;
    await client`insert into ${s}.diag_share_existing (marker) values (${marker})`;
    await client`grant usage on schema ${s} to ${t}`;
    await client`grant select on all tables in schema ${s} to ${t}`;
    await client`alter default privileges in schema ${s} grant select on tables to ${t}`;
    await client`create table if not exists ${s}.diag_share_new (marker text not null)`;
    await client`insert into ${s}.diag_share_new (marker) values (${marker})`;
    return { schema };
  } catch (e) {
    return { schema, error: String(e) };
  } finally {
    await client.end({ timeout: 1 }).catch(() => undefined);
  }
};

// GET /diag/pg/read?schema=<another app's schema>&marker=<m>: whether this app can read the
// marker from that schema's two diag_share tables, and whether it can write there, which a read
// grant must not allow. null when the input is invalid.
export const diagRead = async (schema?: string, marker?: string) => {
  if (!schema || !shareRole.test(schema) || !marker) return null;
  const client = diagConn();
  try {
    const s = client(schema);
    const tables: Record<string, { found: boolean; error: string }> = {};
    for (const table of ["diag_share_existing", "diag_share_new"]) {
      try {
        const [{ found }] = await client<{ found: boolean }[]>`select exists (select 1 from ${s}.${client(table)} where marker = ${marker}) as found`;
        tables[table] = { found, error: "" };
      } catch (e) {
        tables[table] = { found: false, error: String(e) };
      }
    }
    try {
      await client`insert into ${s}.diag_share_existing (marker) values (${marker + "-reader"})`;
      return { tables, wrote: true, write_error: "" };
    } catch (e) {
      return { tables, wrote: false, write_error: String(e) };
    }
  } finally {
    await client.end({ timeout: 1 }).catch(() => undefined);
  }
};
