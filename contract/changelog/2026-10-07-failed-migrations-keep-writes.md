Version 1

`MIGRATE_FAILED` rolls the database back only when the migration changed its definitions, says
which, and names the replaced database (kept for 7 days) where writes made during the migration
can be read. New error `RESTORE_POINT_NOT_ARCHIVED` refuses a restore point that did not reach
the backup archive. The skill says to run each migration in one transaction.
