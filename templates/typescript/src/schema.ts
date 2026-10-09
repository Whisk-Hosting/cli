import { integer, pgTable, serial, text, timestamp } from "drizzle-orm/pg-core";

// One table to start from. Change it, then `npm run generate` writes the next migration into
// drizzle/; `whisk deploy` runs it before traffic switches.
export const notes = pgTable("notes", {
  id: serial("id").primaryKey(),
  authorId: text("author_id").notNull(),
  authorEmail: text("author_email").notNull(),
  body: text("body").notNull(),
  // Filled in by the note-added function, after the note is saved.
  words: integer("words"),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});
