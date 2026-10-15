-- Row-level security (CONTRACT.md §8, SKILL.md §5): each query says who is asking (dbFor and
-- asSystem in src/db.ts), and Postgres keeps every other person's notes out of its answer. Team
-- members and the app's own work see every note, a customer only their own, anything else none.
CREATE INDEX "notes_author_id" ON "notes" ("author_id");--> statement-breakpoint
ALTER TABLE "notes" ENABLE ROW LEVEL SECURITY;--> statement-breakpoint
ALTER TABLE "notes" FORCE ROW LEVEL SECURITY;--> statement-breakpoint
CREATE POLICY "notes_by_audience" ON "notes" USING (
	current_setting('whisk.audience', true) IN ('team', 'system')
	OR "author_id" = nullif(current_setting('whisk.user_id', true), '')
);
