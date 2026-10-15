package main

// The IANA time-zone database, embedded. The manifest's functions[].tz and whisk cron list load
// zones by name, and Windows has no zoneinfo files for Go to find, so without this every zone
// but UTC is "not a known IANA time zone" there. Go reads the system database first when one
// exists; this is the fallback, about 450 KB.
import _ "time/tzdata"
