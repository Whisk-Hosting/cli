# Connections: keypair

`connections.<name>.keypair: {secret: NAME}` asks Whisk to make an EC P-256 key pair for the
connection: the private key becomes the app's secret `NAME`, which the recipe must read (a
check refuses a key pair the recipe never uses), and a person downloads the self-signed
certificate. The connection summary gains `keypair`; `Connection` gains `keypair` with the
certificate, fingerprint and expiry once made.
