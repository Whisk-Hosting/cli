# Only main goes live

`whisk deploy --env production` from a branch pushed that branch over `main` and put it live, so
an agent working on a branch could release it without anyone merging it. Production now runs
only commits the default branch contains. The CLI refuses `--env production` from any branch
other than `main` or `master` before pushing, and `POST /deploys` into production of a commit,
build or preview deploy that `main` does not contain answers the new `PRODUCTION_NEEDS_MAIN`
(409). Rolling back to production's own earlier deploys is unchanged. A preview now says to merge
into `main` and deploy from `main` to put it live. Specs: CONTROL-PLANE.md §6.5 and the deploys
route; CLI.md §5.4; SKILL.md; errors.md; CONTRACT.md §10; HARNESS.md H14.
