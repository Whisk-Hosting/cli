CREATE TABLE "notes" (
	"id" serial PRIMARY KEY NOT NULL,
	"author_id" text NOT NULL,
	"author_email" text NOT NULL,
	"body" text NOT NULL,
	"words" integer,
	"created_at" timestamp with time zone DEFAULT now() NOT NULL
);
