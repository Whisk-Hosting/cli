# Errors: BUCKET_UNREACHABLE and NOT_FOUND on a brought bucket's static files

An app's static files on a bucket its business brought are read by the control plane for the
edge, which never dials that bucket. A file the bucket does not have is `NOT_FOUND` (404); a
bucket that did not answer or refused is `BUCKET_UNREACHABLE` (502), now also on the edge
surface. Neither answer carries the bucket's own words or status.
