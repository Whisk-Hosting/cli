import { defineConfig } from "drizzle-kit";

// `npm run generate` diffs src/db.ts against drizzle/ and writes the next migration.
export default defineConfig({
  dialect: "postgresql",
  schema: "./src/schema.ts",
  out: "./drizzle",
  dbCredentials: { url: process.env.DATABASE_URL ?? "postgres://localhost/unused" },
});
