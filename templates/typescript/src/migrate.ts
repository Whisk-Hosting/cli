// Runs before traffic switches to a new deploy (migrate in whisk.yaml). The platform takes a
// snapshot first and restores it if this exits non-zero.
import { migrate } from "drizzle-orm/postgres-js/migrator";
import { db, sql } from "./db.js";

await migrate(db, { migrationsFolder: "drizzle" });
await sql.end();
console.log("migrations applied");
