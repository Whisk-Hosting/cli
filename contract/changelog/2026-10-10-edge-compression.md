# Headers: the edge compresses text answers

The edge compresses an app's text answers (HTML, CSS, JavaScript, JSON, SVG and the like) with
zstd or gzip for browsers that accept them. An answer the app sends with its own
`Content-Encoding` passes unchanged.
