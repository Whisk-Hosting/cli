# Whisk skill §7: Jobs and workflows in full

The rest of §7 of the Whisk skill (https://skill.whisk.run/), which an agent reads first. Read it when the app runs functions, scheduled jobs, approvals or workflow graphs.

**Approvals.** `approval(step, runId, name, {to, title, data})` is a helper each template ships
(under 20 lines). It sends the event `whisk/approval.requested` in a step named `<name>/request`
with `{approval_id, to, title, data}` where `approval_id` is `<runId>:<name>`, then waits in a
step named `<name>` for `whisk/approval.decided` matching that id, for up to 7 days. The
platform renders the approval page, notifies `to` (an org role such as `owner`, `group:<name>`,
or `user:<email>`) and completes the wait with `{decision, actor, at, note}` where `decision` is
`approved` or `rejected`; a timeout resolves to `{decision: "expired"}`. An owner may always
decide; anyone else only when `to` names them: a role covers that role and the roles above it (`owner` is above all), so
name the lowest role that should decide.

**Graphs.** Each function has a declared graph so humans can see the workflow and the platform
can overlay real runs on it. Step ids are the step names in code.

```yaml
steps:
  - id: fetch-po
  - id: over-limit?
    branches: { yes: request-approval, no: post-po }
  - id: request-approval
    kind: approval
    next: approved?
  - id: approved?
    branches: { yes: post-po, no: end }
  - id: post-po
```

Ids ending `?` are decisions and need not exist in code. Doctor warns when a graph id has no
step in code or a step in code is not in the graph.

**Cron** is `cron:` on a function, in `tz` (default UTC). Nothing runs inside the container on a
timer; sleeping apps wake for the run, and an app nobody visits meanwhile sleeps again about 30
seconds after the run's last step rather than after its plan's idle time. After a deploy, check a cron function without waiting for
its schedule: `whisk cron run <function>` starts one run now, exactly as the schedule would, and
prints the run id to follow with `whisk runs show <id>`. The run's event is
`whisk/cron.run.<function id>` with `data.cron`, so do not branch on the event name. The platform
adds that trigger to every cron function; do not send or listen for `whisk/cron.run.*` yourself.
On the Free plan a schedule runs at most every 10 minutes: `*/5` runs at minutes 0, 10, 20, 30,
40 and 50, and `whisk cron list` shows the schedule each function runs as. Free apps also sleep
after 2 idle minutes and get half a core; one that uses all of it for ten minutes is put to
sleep (`APP_CPU_SLEEP` in its logs), so keep heavy work in small batches.

**Engine features.** It is Inngest itself (v1.46.0): write standard Inngest code with plain
event names (`"po.created"` in triggers, `cancelOn`, `step.waitForEvent` and sends) and
everything works as Inngest documents it, `priority` included.
Event names are your app's own: another app's `po.created` never reaches you. Listen for
failures with `onFailure` (a trigger on `inngest/function.*` never fires), and invoke only your
own app's functions (`INVOKE_FOREIGN`). Runs that sleep or wait count toward your plan's runs in
flight, so set `concurrency` at or below it. The full table, with limits:
https://skill.whisk.run/workflows.md.

**Idempotency.** Runs retry and events can repeat. Key inserts on an id from the event, use
`ON CONFLICT DO NOTHING`, and make every step safe to run twice. Retries default to 3 with
backoff; a run that still fails is parked (`RUN_PARKED`) and the owner is notified; fix and
`whisk runs replay <id>`. `whisk runs list <function>` shows a function's latest runs, cron and
debounced ones included; `--event <id>` narrows to the runs one sent event started.
