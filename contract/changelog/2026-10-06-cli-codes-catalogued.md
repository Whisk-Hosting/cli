Version 1

New catalogue entries for codes the CLI and the node already produced: `DOCTOR_FAILED` (a
failing doctor, its whole report in `details`), `DEPLOY_FAILED` (a failed deploy with no
recorded code), `CLI_ERROR` (a CLI failure with no code of its own), `LOCAL_DOCKER_UNAVAILABLE`
and `DEV_STACK_FAILED` (`whisk dev` without Docker, or with a local stack that would not start),
`WAKE_FAILED` (a sleeping app's container could not be started) and `FORBIDDEN` (the node agent
refusing a caller other than the edge or the mesh). Each error the CLI prints now carries a
`docs` link when its code is catalogued.

W001 reads a `whisk.yaml` saved as UTF-16 or with a UTF-8 byte-order mark, and asks for it to
be saved as UTF-8; one that is not text is reported as such rather than as missing.
