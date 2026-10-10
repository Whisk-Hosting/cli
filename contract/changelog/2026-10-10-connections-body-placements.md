# Connections: auth.body places keys inside a JSON body, and CONNECTION_BODY_UNPLACEABLE

`connections.<name>.auth.body` maps up to eight JSON pointers to templates (secrets and the
token, never the request). The app sends a placeholder at each pointer and the broker replaces
it. A call whose body is not one JSON document, or has no value at a placement, is refused with
`400 CONNECTION_BODY_UNPLACEABLE`. A recipe cannot both place values in the body and sign it.
The connection summary gains `body_fields`.
