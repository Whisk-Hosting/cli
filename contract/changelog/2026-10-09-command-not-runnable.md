# COMMAND_NOT_RUNNABLE

New error code `COMMAND_NOT_RUNNABLE` (deploy, node): the `run` or `migrate` command, or the
image's CMD, could not be run because it is missing, not executable or built for another CPU.
It is the app's to fix; `CONTAINER_START_FAILED` is now only the runtime's own failures.
