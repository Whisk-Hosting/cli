# Coding agents name themselves at login

`POST /device/code` takes `agent`, the coding agent's own name for itself ("Claude Code",
"Codex"), and `whisk login --agent <name>` sends it (else `WHISK_AGENT`, else "Claude Code"
when `CLAUDECODE=1`). `GET /device/{code}` returns it as `requested_from.agent`, and
`POST /device/approve` takes `agent`, the name the person kept or changed. The agent token is
labelled with that name, so deploys read "Claude Code for Sam" rather than "an agent for Sam".
SKILL.md tells every agent to pass `--agent`.
