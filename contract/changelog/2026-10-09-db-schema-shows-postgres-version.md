# The skill points agents at the PostgreSQL version

`contract/SKILL.md` §5 tells agents to check the server's version, which `whisk db schema
--json` prints as `postgres`, before a migration relies on a function from a recent release.
