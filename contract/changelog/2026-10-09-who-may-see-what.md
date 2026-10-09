# Who may see and change what

The skill gains "Who may see and change what" at the end of §4: store each record's owner from
`X-Whisk-User-Id`, filter in the query, check every record fetched by id (404 when the person may
not see it, 403 when they may see but not change it), keep the rules in one place, and test each
kind of record as the wrong person. It also covers the habits that close the other common holes:
parameterised SQL, escaped HTML, keys kept off the browser, checked outbound addresses and
uploads kept in storage.

Every template's `whisk` module carries the rules as pure helpers: `scopeFor`, `inScope`,
`canSee`, `canChange` in TypeScript; `scope_for`, `in_scope`, `can_see`, `can_change` in Python;
`ScopeFor`, `Scope.Includes`, `Identity.CanSee`, `Identity.CanChange` in Go. The team sees
every record, a customer only their own, anyone else none, and changing needs the record's
owner or an owner or admin on the team. Each starter and canary ships property tests that check
the rules over 20,000 generated people and records per rule, including empty and hostile values.

The starters use them: a customer lists only their own notes, and `DELETE /notes/:id` is open
to the note's author as well as owners and admins, answering 404 for a note the caller may not
see.

New doctor warning W094: an app with `customer_identity` whose code never limits what a customer
sees to the signed-in person.
