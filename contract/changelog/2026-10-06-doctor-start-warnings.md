Doctor gains four warnings about start-up, because a sleeping app's visitor waits for it to
start: W100 (the typical start the platform measured is over 3 seconds), W101 (the start
command compiles TypeScript), W102 (migrations run on every start) and W103 (the entry file
loads a heavy library at its top level). The app JSON gains `start` {typical_ms, count}, the
deploy JSON and the deploy's last event `start_ms`, and `/apps/:app/validate` the app's `start`.
The skill's "Do not" list says not to make the app slow to start. Additive: nothing existing
changed.
