Version 1

The skill says what removing an app customer means for the app: they are signed out and cannot
sign in, and if they are invited again (or sign up again) within 30 days they come back under the
same `X-Whisk-User-Id`, so an app keeps their records keyed by it. After 30 days the platform
deletes the person and a later invitation makes a new id. No field, header or code changes.
