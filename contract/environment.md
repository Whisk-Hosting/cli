# Environment

Every variable the platform injects into an app's process. Names are stable within a conventions
version. Read them from the environment at start; never commit values; never rename them.

## Always present

| Variable | Value |
|---|---|
| `PORT` | `8080`. Listen here, on all interfaces. |
| `WHISK_APP_ID` | ULID of the app |
| `WHISK_APP_NAME` | the manifest `name` |
| `WHISK_ORG_ID` | ULID of the org |
| `WHISK_ENV` | `production` or `preview:<branch>` |
| `WHISK_PUBLIC_URL` | `https://<hostname>` for this environment |
| `WHISK_QUEUE_URL` | `https://api.whisk.run/v1/orgs/<org>/apps/<app>/events`; POST events here |
| `WHISK_SERVICE_TOKEN` | bearer token for `WHISK_QUEUE_URL`, the uploads, customers, email and custom domain endpoints, and app-to-app calls. Rotated daily; read it per request, never cache it in a file. |
| `WHISK_INNGEST_URL` | origin of the workflow API the Inngest SDK talks to (scheme and host, no path); pass it as the SDK's base URL |
| `WHISK_INNGEST_SIGNING_KEY` | this app's own; verifies function-run requests to `queue.endpoint`, and tells the platform which app the SDK is |
| `WHISK_INNGEST_EVENT_KEY` | this app's own; lets the Inngest SDK send events from inside a function |
| `WHISK_DELIVERY_KEY` | base64, 32 bytes, this app's own; verifies `X-Whisk-Delivery-Signature` on webhook deliveries (headers.md, delivery headers) |
| `SENTRY_DSN` | error tracking; initialise any Sentry SDK with it |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | `https://api.whisk.run/v1/otlp/v1/traces`, where an OpenTelemetry SDK sends the app's traces |
| `OTEL_EXPORTER_OTLP_TRACES_HEADERS` | `Authorization=Bearer%20<service token>`, the exporter's header; a credential, scrubbed from logs |
| `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` | `http/protobuf` |
| `OTEL_SERVICE_NAME` | the app's name |
| `OTEL_RESOURCE_ATTRIBUTES` | `deployment.environment=<WHISK_ENV>`, percent-encoded |
| `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER` | `none`: the platform keeps traces, and the app's stdout is its logs |
| `WHISK_STOP_GRACE` | seconds between SIGTERM and SIGKILL, default `28` |
| `WHISK_MEMORY_BYTES` | the app's memory limit in bytes, set by the org's plan (Free 268435456, 256 MiB). Above it the process is killed at once, with no SIGTERM, and the next run's log starts with `APP_OUT_OF_MEMORY`; at 85 % the log shows `APP_MEMORY_HIGH`. Absent only when there is no limit (`whisk dev`). |
| `WHISK_TMP_BYTES` | the size of `/tmp` in bytes. `/tmp` is held in memory, so what the app keeps there counts against `WHISK_MEMORY_BYTES`: a run that fills `/tmp` has that much less for the process. |
| `TZ` | `UTC` |

## Present by manifest

| Variable | When | Value |
|---|---|---|
| `DATABASE_URL` | `database` is `app` or `shared:<name>` | `postgres://…?sslmode=require`, pooled in transaction mode. For `shared:`, the app's own schema is first on `search_path`. With `database_role: restricted` the running app's is the run login, `<owner>_run`, which reads and writes rows and cannot change the schema; the `migrate` step's is the owner over a direct connection. |
| `WHISK_STORAGE_ENDPOINT`, `WHISK_STORAGE_BUCKET`, `WHISK_STORAGE_ACCESS_KEY`, `WHISK_STORAGE_SECRET_KEY`, `WHISK_STORAGE_PREFIX`, `WHISK_STORAGE_REGION` | `storage: true` | S3-compatible credentials for the org's bucket, which is the tenant boundary. Write under the prefix, which is your app's own. Use path-style addressing; the endpoint is reachable from the container and from browsers. |
| `WHISK_KV_URL` | `kv: true` | `redis://…` to a Valkey the app has to itself: no prefix, no sharing. Empty after the app sleeps; sized by the plan, and full means the app's own least recently used keys go. |
| `WHISK_CONNECTION_<NAME>_URL` | each entry under `connections` | `http://connect.internal.whisk:8443/<name>`, the broker's address for that connection. The connection's secrets are never set. |
| every name in `secrets:` | always | the value a human set; absent until then |
| every name in `env:` | always | the literal value from the manifest |
| `WHISK_PAUSED` | a managed app a person paused | `true`; the app does no work on a run and answers its routes 503 `APP_PAUSED` until it is resumed |
| `WHISK_LINKED_<ROLE>` | a managed app with a link | the linked app's id, one per role the product names, e.g. `WHISK_LINKED_SHOP` |
| `WHISK_LINKED_<PRODUCT>` | an app a managed app is linked to | the copy's id, e.g. `WHISK_LINKED_ERP_LINK`; set from the app's next start or deploy after the link is made |
| a managed app's variant `env` and settings | a managed app | the variant's literal values and what the business set |

Start any OpenTelemetry SDK's OTLP/HTTP trace exporter and it reads the `OTEL_*` variables
itself; the templates do this only when `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` is set. A name the
manifest sets under `env` or `secrets` keeps the manifest's value, so an app can send its traces
to a service of its own instead. Traces are kept 7 days and read with `whisk traces` and on the
app's Traces page.

## Local development

`WHISK_DEV=1` is set only by `whisk dev` and by the stub, which set no `OTEL_*` variable. Apps may use it to relax things that
only make sense in production, for example Sentry sampling. Nothing else changes: the same
headers arrive, the same variables exist.

## Reserved

Any name beginning `WHISK_` is reserved for the platform. The manifest refuses them in `secrets`
and `env`. The Inngest SDKs also read `INNGEST_*` names; the templates set the SDK's options from
the `WHISK_INNGEST_*` variables in code rather than relying on that.
