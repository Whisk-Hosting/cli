Version 1

One error code: `DEPLOY_IN_PROGRESS`, from `whisk deploy` when it would wait on a deploy of the same
environment that has sat queued or building for 25 minutes or more. It names the deploy, how long it
has waited and `whisk deploys cancel <id>`. A deploy nothing on Whisk's side is still working on is
ended as `PLATFORM_DEPLOY_FAILED` with cause `PLATFORM_UNAVAILABLE` and `details.step` `build` or
`queue`. A `git push` that drops part-way ("the remote end hung up") is `PLATFORM_UNAVAILABLE` like
any other lost connection. `POST /deploys {commit_sha}` for a commit with no build that succeeded
builds it again instead of answering `NOT_FOUND`.
