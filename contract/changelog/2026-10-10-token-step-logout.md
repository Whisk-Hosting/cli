# Token steps log their sessions out

A session login holds one of the business's licences until it times out (SAP Business One), so a
broker that let go of a token left a licence taken for up to half an hour. A token step may now
name `logout`: a POST, GET or DELETE on the token step's own host, carrying the token in the
call's own headers or in headers of its own, with the kept cookies. The broker sends it whenever
it lets go of a token that has not expired: when it fetches a fresher one, when the grant is
given again, on a pause or a revoke, and when it stops (within 10 seconds, after calls in flight
finish). It is never sent after a 401. On a revoke, a token with no revocation endpoint counts as
cancelled when its logout is accepted, and the grant screen says revoking cancels the tokens.
(BROKER.md §3 "Logging out"; CONTRACT.md §3.1.)
