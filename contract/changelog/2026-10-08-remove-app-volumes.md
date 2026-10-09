# Volumes leave the contract

`TrustBackups` no longer carries `last_volume_at`, and `ResourceExport` no longer has
`volumes`: no app could declare a volume, so none was ever backed up or exported. The error
catalogue drops `VOLUME_FAILED` and `RESTIC_MISSING` and adds `NODE_STATE_FAILED`, for a node
that could not write to its own working directory while scanning, converting or resizing a file.
