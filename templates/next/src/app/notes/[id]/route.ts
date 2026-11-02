import { eq } from "drizzle-orm";
import { dbFor, notes } from "../../../db";
import { logged, who } from "../../../request";
import { canChange, canSee } from "../../../whisk";

export const DELETE = logged<{ id: string }>(async (req, { params }) => {
  const id = who(req);
  const noteId = Number((await params).id);
  if (!Number.isSafeInteger(noteId) || noteId < 1 || noteId > 2147483647) return Response.json({ error: "no such note" }, { status: 404 });
  return dbFor(id, async (tx) => {
    const [note] = await tx.select().from(notes).where(eq(notes.id, noteId));
    // A note the person may not see is answered as if it did not exist.
    if (!note || !canSee(id, note.authorId)) return Response.json({ error: "no such note" }, { status: 404 });
    if (!canChange(id, note.authorId)) return Response.json({ error: "only its author or an owner or admin can delete it" }, { status: 403 });
    await tx.delete(notes).where(eq(notes.id, note.id));
    return new Response(null, { status: 204 });
  });
});
