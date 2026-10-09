# whisk.yaml: network

`network: internal | public` says where a Whisk On-Premise app is served: on the company's
network only (the default there), or also on the outside address. whisk.run serves every app
publicly and refuses `internal` with `MANIFEST_INVALID`. Leaving it out changes nothing.
