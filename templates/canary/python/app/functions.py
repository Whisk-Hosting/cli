"""Three functions, one per trigger kind the platform offers. Step names match the ids in
workflows/*.graph.yaml; doctor checks that they do."""

import datetime as dt
from typing import Any

import inngest

from .db import count_notes, record_event
from .whisk import approval, client, log


@client.create_function(fn_id="nightly-summary", trigger=inngest.TriggerCron(cron="0 6 * * *"))
def nightly_summary(ctx: inngest.ContextSync) -> dict[str, Any]:
    total = ctx.step.run("count-notes", count_notes)
    day = dt.date.today().isoformat()
    return ctx.step.run("record-summary", record_event, f"summary:{day}", "summary", {"day": day, "notes": total})


@client.create_function(fn_id="canary-event", trigger=inngest.TriggerEvent(event="canary.event"), retries=3)
def canary_event(ctx: inngest.ContextSync) -> dict[str, Any]:
    data = ctx.step.run("receive", lambda: {**ctx.event.data, "run_id": ctx.run_id})

    def compute() -> dict[str, Any]:
        if data.get("fail"):
            raise RuntimeError("canary.event asked to fail")
        numbers = data.get("numbers") if isinstance(data.get("numbers"), list) else []
        return {**data, "sum": sum(numbers)}

    result = ctx.step.run("compute", compute)
    return ctx.step.run("record", record_event, ctx.run_id, "run", result)


@client.create_function(fn_id="canary-approval", trigger=inngest.TriggerEvent(event="canary.approval"))
def canary_approval(ctx: inngest.ContextSync) -> Any:
    decision = approval(ctx.step, ctx.run_id, "request-approval", to="owner", title=f"Canary approval for run {ctx.run_id}", data=ctx.event.data)
    if decision.get("decision") == "approved":
        return ctx.step.run("record-decision", record_event, f"{ctx.run_id}:decision", "approval", {**decision, **ctx.event.data})

    def notify() -> str:
        log.info("approval not granted", run_id=ctx.run_id, decision=decision)
        return str(decision.get("decision"))

    return ctx.step.run("notify-rejection", notify)


functions = [nightly_summary, canary_event, canary_approval]
