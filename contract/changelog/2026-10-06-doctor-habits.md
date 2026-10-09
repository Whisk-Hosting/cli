Version 1

New doctor warnings W004 (a static folder the commit will not carry), W005 (health.timeout above
the 60-second wake), W023 (a timer or scheduler in the app), W024 (session state on the pooled
connection), W043 (a declared function with no matching id in code), W064 (a build secret the
Dockerfile does not mount), W091 (a cookie Domain), W092 (identity from Authorization or a
self-verified token) and W093 (an outside service for sign-in, email or a cache). A
`doctor: allow <rule>` comment on or above the line keeps a deliberate warning out of the report.
