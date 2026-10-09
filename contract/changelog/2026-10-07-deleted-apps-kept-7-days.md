Version 1

A deleted app keeps its data for 7 days: `whisk apps deleted` lists it and `whisk apps restore
<id>` brings it back stopped. New error `APP_NOT_RESTORABLE` answers a restore that is too late or
whose slug is taken. The skill names the commands.
