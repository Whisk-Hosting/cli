// The app's functions: work that runs on the platform's queue, outside a request, retried step
// by step. Each is declared under functions in whisk.yaml with its graph in workflows/; step
// names match the graph's ids, and doctor checks that they do.
import { eq } from "drizzle-orm";
import { db, notes } from "./db.js";
import { inngest } from "./whisk.js";

// note-added runs for every note.added event POST /notes sends. Each step runs once and its
// result is kept, so a retry resumes after the last finished step; both steps are safe to run
// twice.
export const noteAdded = inngest.createFunction(
  { id: "note-added", triggers: [{ event: "note.added" }], retries: 3 },
  async ({ event, step }) => {
    const id = Number((event.data as { note_id?: unknown }).note_id);
    const words = await step.run("count-words", async () => {
      const [note] = await db.select({ body: notes.body }).from(notes).where(eq(notes.id, id));
      return note ? note.body.split(/\s+/).filter(Boolean).length : null;
    });
    if (words === null) return { note_id: id, found: false };
    await step.run("save-count", () => db.update(notes).set({ words }).where(eq(notes.id, id)));
    return { note_id: id, words };
  },
);

export const functions = [noteAdded];
