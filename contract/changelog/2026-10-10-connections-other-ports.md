# Connections reach HTTPS on ports other than 443

SAP Business One's Service Layer listens on 50000, and some hosted Acumatica, Odoo and Business
Central installs use their own port. A connection's address (and its token step's) may now name
port 443 or any port from 1024 to 65535 (`connect.PortAllowed`); the broker sends there with the
same rules as before: the granted host, a public address checked on the address dialled, TLS
verified against the host name, no redirects. Ports below 1024 other than 443 stay refused, so a
connection cannot be pointed at mail or another well-known service. The grant screen already
shows the port, and a different port is a different recipe, so changing it needs a new grant.
(BROKER.md §2 step 7, CONTRACT.md §3.1; ERP link gap 4.)
