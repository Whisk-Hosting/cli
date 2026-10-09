"""The app's functions: work that runs on the platform's queue, outside a request, retried step
by step. Each is declared under functions in whisk.yaml with its graph in workflows/; step names
match the graph's ids, and doctor checks that they do."""

from typing import Any

import inngest

from .db import note_body, set_words
from .whisk import client


@client.create_function(fn_id="note-added", trigger=inngest.TriggerEvent(event="note.added"), retries=3)
def note_added(ctx: inngest.ContextSync) -> dict[str, Any]:
    """Runs for every note.added event POST /notes sends. Each step runs once and its result is
    kept, so a retry resumes after the last finished step; both steps are safe to run twice."""
    note_id = int(ctx.event.data.get("note_id", 0))

    def count_words() -> int:
        body = note_body(note_id)
        return -1 if body is None else len(body.split())

    words = ctx.step.run("count-words", count_words)
    if words < 0:
        return {"note_id": note_id, "found": False}
    ctx.step.run("save-count", set_words, note_id, words)
    return {"note_id": note_id, "words": words}


functions = [note_added]
