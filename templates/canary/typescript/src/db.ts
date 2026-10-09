import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import { events, notes } from "./schema.js";
import { env, traceQueries } from "./whisk.js";

export { events, notes };

// DATABASE_URL is pooled in transaction mode: no prepared statements across statements.
export const sql = traceQueries(postgres(env("DATABASE_URL"), { prepare: false, max: 5 }));
export const db = drizzle(sql, { schema: { notes, events } });

// recordEvent inserts once per id; verified marks a webhook delivery the deliveries helper
// proved to be the platform's.
export const recordEvent = async (id: string, kind: string, payload: unknown, verified = false) => {
  const inserted = await db.insert(events).values({ id, kind, payload, verified }).onConflictDoNothing().returning({ id: events.id });
  return { id, kind, duplicate: inserted.length === 0 };
};
