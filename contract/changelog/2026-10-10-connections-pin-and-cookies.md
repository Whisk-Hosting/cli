# Connections: pin and auth.token.cookies

`connections.<name>.pin` (`sha256/` and the base64 SHA-256 of a certificate's public key info)
makes the broker accept only that key from the connection's host, in place of public roots and
the host name. `auth.token.cookies` names up to four cookies the token answer sets, which the
broker sends on every call and on the revocation. The connection summary gains `pin`.
