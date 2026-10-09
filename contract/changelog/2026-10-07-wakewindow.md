Version 1

New package `wakewindow`: the edge's wake confirmation span (60 seconds), its grace while a
node agent cannot be reached (5 minutes) and the node's subnet quarantine (10 minutes). The edge
and the node agent both read them, and a test keeps the grace at least 5 minutes inside the
quarantine (CADDY.md §5.2, NODE-AGENT.md §A5). Nothing an app builds against changes.
