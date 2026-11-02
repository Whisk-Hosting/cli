import type { APIRoute } from "astro";
import { eq } from "drizzle-orm";
import { dbFor, notes } from "../../db.js";
import { canChange, canSee, identity } from "../../whisk.js";

export const DELETE: APIRoute = async ({ request, params }) => {
  const id = identity((n) => request.headers.get(n) ?? undefined);
  const noteId = Number(params.id);
  if (!Number.isSafeInteger(noteId) || noteId < 1 || noteId > 2147483647) return Response.json({ error: "no such note" }, { status: 404 });
  return dbFor(id, async (tx) => {
    const [note] = await tx.select().from(notes).where(eq(notes.id, noteId));
    // A note the person may not see is answered as if it did not exist.
    if (!note || !canSee(id, note.authorId)) return Response.json({ error: "no such note" }, { status: 404 });
    if (!canChange(id, note.authorId)) return Response.json({ error: "only its author or an owner or admin can delete it" }, { status: 403 });
    await tx.delete(notes).where(eq(notes.id, note.id));
    return new Response(null, { status: 204 });
  });
};
