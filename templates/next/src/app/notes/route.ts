import { desc, eq } from "drizzle-orm";
import { dbFor, notes } from "../../db";
import { logged, who } from "../../request";
import { enqueue, log, scopeFor } from "../../whisk";

export const dynamic = "force-dynamic";

// Private: the platform signs people in before a request reaches it. Each note belongs to its
// author (authorId). The team sees every note; a customer, when the app has customer_identity,
// sees only their own (whisk.ts scopeFor), and the table's row-level security keeps to the same
// rule should a query forget it (dbFor).
export const GET = logged(async (req) => {
  const id = who(req);
  const scope = scopeFor(id);
  if (scope.kind === "none") return Response.json([]);
  const mine = scope.kind === "owner" ? eq(notes.authorId, scope.ownerId) : undefined;
  return Response.json(await dbFor(id, (tx) => tx.select().from(notes).where(mine).orderBy(desc(notes.id)).limit(100)));
});

export const POST = logged(async (req) => {
  const id = who(req);
  const authorId = id.userId;
  if (!authorId) return Response.json({ error: "a signed-in person is required" }, { status: 403 });
  const text = ((await req.json().catch(() => ({}))) as { body?: string }).body?.trim();
  if (!text) return Response.json({ error: "body is required" }, { status: 400 });
  const [note] = await dbFor(id, (tx) => tx.insert(notes).values({ authorId, authorEmail: id.email ?? "", body: text }).returning());
  // Hand the slow part to a function. A preview runs no functions, so it sends no events.
  if (process.env.WHISK_QUEUE_URL && !(process.env.WHISK_ENV ?? "").startsWith("preview:")) {
    await enqueue("note.added", { note_id: note.id }, `note-${note.id}`).catch((err: unknown) =>
      log.warn({ request_id: id.requestId, note_id: note.id, err: String(err) }, "note.added not sent"),
    );
  }
  return Response.json(note, { status: 201 });
});
