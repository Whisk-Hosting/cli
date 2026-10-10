# Connection operations match on the JSON body

A JSON-RPC API sends every call to one path and names the model and method inside the body, so a
grant of `POST /jsonrpc` would have let the app call anything the outside system's user may. An
operation may now name `body`: up to four JSON pointers, each with the string the call's body
must hold there. The broker reads such a body (up to 1 MiB) as one JSON document before
matching, the operation matches only when each pointer holds exactly its string, and the broker
sends the document written again from what it read, so the outside system parses what was
matched. Body conditions are part of the operation a person grants: changing them under the same
name is a new operation, marked New on the grant screen, which shows each condition under the
path. (BROKER.md §2 step 3; CONTRACT.md §3.1; DASHBOARD.md.)
