// Three functions, one per trigger kind the platform offers. Step names match the ids in
// workflows/*.graph.yaml; doctor checks that they do.
import { count } from "drizzle-orm";
import { asSystem, notes, recordEvent } from "./db.js";
import { approval, inngest, log } from "./whisk.js";

export const nightlySummary = inngest.createFunction(
  { id: "nightly-summary", triggers: [{ cron: "0 6 * * *" }] },
  async ({ step }) => {
    const total = await step.run("count-notes", async () => (await asSystem((tx) => tx.select({ n: count() }).from(notes)))[0].n);
    const day = new Date().toISOString().slice(0, 10);
    return step.run("record-summary", () => recordEvent(`summary:${day}`, "summary", { day, notes: total }));
  },
);

export const canaryEvent = inngest.createFunction(
  { id: "canary-event", triggers: [{ event: "canary.event" }], retries: 3 },
  async ({ event, step, runId }) => {
    const data = await step.run("receive", (): Record<string, unknown> => ({ ...(event.data as Record<string, unknown>), run_id: runId }));
    const result = await step.run("compute", () => {
      if (data.fail) throw new Error("canary.event asked to fail");
      const numbers = Array.isArray(data.numbers) ? (data.numbers as number[]) : [];
      return { ...data, sum: numbers.reduce((a, b) => a + b, 0) };
    });
    return step.run("record", () => recordEvent(runId, "run", result));
  },
);

export const canaryApproval = inngest.createFunction(
  { id: "canary-approval", triggers: [{ event: "canary.approval" }] },
  async ({ event, step, runId }) => {
    const decision = await approval(step, runId, "request-approval", {
      to: "owner",
      title: `Canary approval for run ${runId}`,
      data: event.data,
    });
    if (decision.decision === "approved") {
      return step.run("record-decision", () => recordEvent(`${runId}:decision`, "approval", { ...decision, ...(event.data as object) }));
    }
    return step.run("notify-rejection", () => {
      log.info({ run_id: runId, decision }, "approval not granted");
      return decision.decision;
    });
  },
);

export const functions = [nightlySummary, canaryEvent, canaryApproval];
