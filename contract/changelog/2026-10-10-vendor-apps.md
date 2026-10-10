# Whisk's vendor apps

Xero, MYOB and Microsoft expect one developer app per integrator, which no shop owner should have
to register. An operator now sets Whisk's own (client ID, secret, optional webhook key and the
vendor's limits for the whole app) on the operator page's Setup, sealed and never shown again. A
connection's recipe names one at `auth.vendor_app` and reads `{vendor.client_id}` and
`{vendor.client_secret}`; only an app Whisk manages may grant or call such a connection
(`CONNECTION_VENDOR_APP_REFUSED`), and another app's deploy declaring one fails at once with
that code rather than waiting on a grant nobody can give. Every call counts against the vendor app's shared
per-second, per-minute and per-day limits across every business, so one busy business cannot
spend Whisk's MYOB allowance for the rest. The grant screen says the connection signs in as
Whisk's app. (CONTROL-PLANE.md §6.8 "Whisk's vendor apps"; CONTRACT.md §3.1; BROKER.md §2;
DASHBOARD.md Setup.)
