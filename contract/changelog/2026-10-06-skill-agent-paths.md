# 2026-10-06: The skill matches what agents meet

`SKILL.md` documents the login resume flow, following deploys and restores, the restore window per
plan, deploy warnings, the exit codes as the CLI returns them, the Windows install from Git Bash,
`whisk dev`, the `whisk-stub` path, and adds FORBIDDEN_ROLE, AGENT_CATEGORY, AGENT_PAUSED,
CONTAINER_CRASHED and FUNCTIONS_NOT_REGISTERED to its table of codes. It names which
templates run on a distroless image (TypeScript and Go; Python runs on `python:3.13-slim`), and the
`whisk.yaml` examples in CONTRACT.md and CONTROL-PLANE.md use a plain `migrate` command.
