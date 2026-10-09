Version 1

New catalogue entry `FUNCTIONS_NOT_REGISTERED`: a warning on a live deploy whose functions the
workflow engine could not register, naming the functions or the engine's error.

`HEALTH_CHECK_FAILED` documents `path`, `timeout_seconds` and a numeric `last_status`;
`CONTAINER_CRASHED` says when it is a crash at start and when one after going live, and which
carries the exit code; `SECRET_UNSET` covers `build.secrets`; `SECRET_TOO_SHORT` applies to every
setter; `MANIFEST_INVALID` covers a static folder missing from the commit; `MIGRATE_FAILED` names
its 600-second limit and `restore_point`. `UPDATE_FAILED`, `EXPORT_FAILED`, `INVALID_ARGUMENT`,
`DATABASE_FAILED` and `RESTORE_FAILED` name `whisk feedback --kind bug --code <CODE>` as the way
to report. `CONFIRM_REQUIRED` carries `details.restorable`; `RESTORE_IN_PROGRESS` names
`whisk restore show <id> --wait`.

`previews.database` accepts only `empty`: `clone` is refused with `MANIFEST_INVALID`, because no
preview copies production's data.
