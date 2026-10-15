import { boolean, jsonb, pgTable, serial, text, timestamp } from "drizzle-orm/pg-core";

export const notes = pgTable("notes", {
  id: serial("id").primaryKey(),
  authorId: text("author_id").notNull(),
  authorEmail: text("author_email").notNull(),
  body: text("body").notNull(),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});

// events records what functions and webhooks did, keyed by the id the platform gave us, so a
// repeat (retry, replay, duplicate event) is a no-op insert. verified is true on a webhook
// delivery the deliveries helper proved to be the platform's.
export const events = pgTable("events", {
  id: text("id").primaryKey(),
  kind: text("kind").notNull(),
  payload: jsonb("payload").notNull(),
  verified: boolean("verified").notNull().default(false),
  createdAt: timestamp("created_at", { withTimezone: true }).notNull().defaultNow(),
});
