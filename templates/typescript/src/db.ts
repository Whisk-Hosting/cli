import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";
import { notes } from "./schema.js";
import { env, traceQueries } from "./whisk.js";

export { notes };

// DATABASE_URL is pooled in transaction mode: no prepared statements across statements.
export const sql = traceQueries(postgres(env("DATABASE_URL"), { prepare: false, max: 5 }));
export const db = drizzle(sql, { schema: { notes } });
