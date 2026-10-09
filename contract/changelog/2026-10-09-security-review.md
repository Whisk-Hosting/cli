# Ambiguous paths, `.whisk/dev` and committed bindings

The error catalogue adds `PATH_AMBIGUOUS` (400, edge and auth) for a request path with a `.` or
`..` segment, which the local stub edge also refuses. Doctor rule W011 also flags tracked files
under `.whisk/dev/`, and the new warning W006 flags a committed `.whisk/app.json` that binds a
different app than the `whisk` remote. The schema's `static[].dir` and `functions[].graph`
patterns no longer allow a `..` segment, an empty segment or a leading `/`. SKILL.md says that
`whisk dev` runs the migrate command on the local machine.
