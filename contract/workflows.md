# Workflow features

Whisk runs functions on Inngest itself, self-hosted: engine **v1.46.0**. Write standard Inngest
code with plain event names and every feature below works as Inngest documents it. Every row below was tested against that engine through
the platform's own event path with the TypeScript SDK the template pins (`inngest` 4.20); the
Python and Go SDKs speak the same protocol to the same engine. The harness scenario H35 checks
the rows marked **H35** against production every night.

One engine serves every app, so the platform keeps each app to itself. None of this changes the
code you write:

1. **Event names are your app's own.** Write `"po.created"` everywhere: triggers, `cancelOn`,
   `step.waitForEvent`, `step.sendEvent`, `inngest.send` and `POST $WHISK_QUEUE_URL`. The
   platform files each event under your app and hands it back to your code under the name you
   wrote, so two apps can both use `po.created` without hearing each other.
2. **Your app hears only its own events.** A trigger on the engine's `inngest/function.finished`
   or `inngest/function.failed` never fires; use `onFailure` for failures.
3. **Your app runs only its own functions.** `step.invoke` works on your app's functions; an
   invocation of another app's function is refused with `INVOKE_FOREIGN`. Call another app's
   service route instead.

## Support table

| Feature | Works | Notes |
|---|---|---|
| Event trigger | yes, **H35** | `triggers: [{ event: "po.created" }]` |
| Trigger `if` expression | yes | `{ event, if: "event.data.total > 100" }` filters before a run starts |
| Several triggers on one function | yes | |
| Wildcard trigger | yes | `"po.*"` |
| Cron trigger | yes | `cron:` in `whisk.yaml` with `tz`, or `"TZ=Pacific/Auckland 0 6 * * *"` in code |
| Run a cron function now | yes | `whisk cron run <function>`; the event is `whisk/cron.run.<function id>` with `data.cron`, a trigger the platform adds to every cron function; apps may not send or listen for that family |
| Retries | yes | default 3 retries (4 attempts) with the engine's backoff |
| `NonRetriableError` | yes | the run fails at once |
| `RetryAfterError` | yes | the retry waits at least the time given |
| `onFailure` | yes, **H35** | runs once after the last attempt, with the error and the original event |
| Trigger on `inngest/function.failed` or `.finished` | no | never fires; use `onFailure` |
| `step.run` memoisation | yes | code between steps runs again on every resume |
| Parallel steps (`Promise.all`) | yes | |
| `step.sleep`, `step.sleepUntil` | yes | a sleeping run counts toward your plan's runs in flight |
| `step.waitForEvent` | yes, **H35** | `match: "data.id"` pairs it with the run's event |
| `step.sendEvent` / fan-out | yes | |
| `step.invoke` | yes, **H35** | your own app's functions only |
| `cancelOn` | yes, **H35** | `match: "data.id"` or `if: "async.data.id == event.data.id"` |
| `timeouts.finish` | yes | the run ends `cancelled` |
| `concurrency` (limit, per `key`, several limits) | yes, **H35** | extra runs queue, none are dropped |
| `concurrency` with `scope: "env"` or `"account"` | yes | shared across your app's functions only |
| `throttle` | yes | spaces runs out, drops none |
| `rateLimit` | yes | runs over the limit are dropped |
| `debounce` | yes, **H35** | one run, with the last event |
| `singleton` (`skip`, `cancel`) | yes | |
| `idempotency` | yes | one run per key per 24 hours |
| Event `id` | yes | one run per function per id per 24 hours; the queue URL's `dedupe_key` does the same |
| `batchEvents` (with `key`) | yes | flushes at `maxSize` or `timeout` |
| `priority.run` | yes, **H35** | seconds, -43200 to 43200: a run with 600 starts ahead of runs queued up to 10 minutes before it, one with -600 waits behind runs queued up to 10 minutes after it; it matters only while runs are waiting, such as behind a `concurrency` limit. `"event.data.boost"` works: the platform makes the expression a whole number, which the engine needs |
| Checkpointing (SDK v4) | yes | faster runs; a refused checkpoint falls back to one call per step |

## Limits the platform adds

- **Runs in flight.** A function with more running runs than your plan's `concurrent_runs`
  (Free 5, Team and Agency 20, Business 50) is parked with `RUN_CONCURRENCY_LIMIT`. Runs that
  are sleeping or waiting for an event count. Set `concurrency: { limit: N }` at or below your
  plan's number so a burst queues instead of parking the function.
- **Steps per run.** A run past 1,000 steps is cancelled with `RUN_STEP_LIMIT`.
- **Events per minute.** Your plan's rate; more are refused with `EVENT_RATE_LIMITED`.
- **Run state.** The engine sends each call the event and every step's output, up to 64 MB in
  all; keep outputs small and store large data in the database or the storage bucket.
- **`whisk.yaml`.** `concurrency` and `retries` on a function in the manifest describe it for
  people; the options in code are what the engine runs, so keep them the same.
- **Names you already prefixed.** Code written for Whisk before names were plain uses
  `<WHISK_APP_ID>/po.created` in triggers. It keeps working unchanged. An app gets plain names
  only when none of its triggers and cancellations carries the prefix, so do not mix the two:
  in an app with one prefixed trigger, a plain trigger never fires.
