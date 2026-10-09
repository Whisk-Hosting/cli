import { sql as q } from "drizzle-orm";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import { events, notes } from "./schema.js";
import { env, type Identity, traceQueries } from "./whisk.js";

export { events, notes };

// DATABASE_URL is pooled in transaction mode: no prepared statements across statements.
export const sql = traceQueries(postgres(env("DATABASE_URL"), { prepare: false, max: 5 }));
export const db = drizzle(sql, { schema: { notes, events } });

export type Tx = Parameters<Parameters<typeof db.transaction>[0]>[0];

// Who a query runs as: the request's audience and person, or "system" for the app's own work.
export type Caller = { audience: Identity["audience"] | "system"; userId?: string };

// The two settings the row-level security policies read (drizzle/0002_row_level_security.sql).
// Pure.
export const callerSettings = (c: Caller) => ({ audience: c.audience, userId: c.userId ?? "" });

// dbFor runs fn in one short transaction that first tells Postgres who is asking, so a table
// with a policy answers only that caller's rows even when a query forgets its filter. A query
// on db itself says nothing, and those tables answer it with no rows. Keep slow work (calls to
// other services) outside fn: the transaction holds a pooled connection until it ends.
export const dbFor = <T>(caller: Caller, fn: (tx: Tx) => Promise<T>): Promise<T> =>
  db.transaction(async (tx) => {
    const s = callerSettings(caller);
    await tx.execute(q`select set_config('whisk.audience', ${s.audience}, true), set_config('whisk.user_id', ${s.userId}, true)`);
    return fn(tx);
  });

// asSystem is dbFor for the app's own work, outside any person's request: functions, webhook
// deliveries. It sees every row, so never use it to answer a person.
export const asSystem = <T>(fn: (tx: Tx) => Promise<T>): Promise<T> => dbFor({ audience: "system" }, fn);

// recordEvent inserts once per id; verified marks a webhook delivery the deliveries helper
// proved to be the platform's.
export const recordEvent = async (id: string, kind: string, payload: unknown, verified = false) => {
  const inserted = await db.insert(events).values({ id, kind, payload, verified }).onConflictDoNothing().returning({ id: events.id });
  return { id, kind, duplicate: inserted.length === 0 };
};
