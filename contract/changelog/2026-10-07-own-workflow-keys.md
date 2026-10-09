# The workflow keys are the app's own

`WHISK_INNGEST_SIGNING_KEY` and `WHISK_INNGEST_EVENT_KEY` (environment.md) are this app's own,
made for it by the platform, and tell the platform which app the SDK is. Code reads them as
before; nothing in an app changes.
