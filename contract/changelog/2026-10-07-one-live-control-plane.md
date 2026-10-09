Version 1

# CONTROL_PLANE_SUPERSEDED and ROLLBACK_UNAVAILABLE

New node code `CONTROL_PLANE_SUPERSEDED`: a node refused a command from a control plane copy
that is no longer live, because its epoch is older than the highest the node has seen
(NODE-AGENT.md §A13). New API code `ROLLBACK_UNAVAILABLE`: an operator's rollback of the control
plane could not happen, with the reason. Additive: no code changed.
