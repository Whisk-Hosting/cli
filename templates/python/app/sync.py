"""Working files for batch jobs (CONTRACT.md section 8).

The container's disk is /tmp, which is emptied on every restart, so a job that keeps a tree of
files between runs keeps it in the app's bucket and works on a copy: pull_tree before the run,
push_tree during and after it. Each tree has an index of every file's SHA-256 beside it, so a
pull downloads only what differs from /tmp and a push uploads only what changed. Keep one run
per tree at a time with a lease row in the database. Needs `storage: true`.
"""

import hashlib
import json
import os
from pathlib import Path

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError


def storage_client():
    """The bucket client, or None when the manifest did not ask for storage."""
    endpoint = os.environ.get("WHISK_STORAGE_ENDPOINT")
    if not endpoint:
        return None
    return boto3.client(
        "s3",
        endpoint_url=endpoint,
        region_name=os.environ.get("WHISK_STORAGE_REGION") or "us-east-1",
        aws_access_key_id=os.environ.get("WHISK_STORAGE_ACCESS_KEY", ""),
        aws_secret_access_key=os.environ.get("WHISK_STORAGE_SECRET_KEY", ""),
        # The bucket goes in the path: storage.<domain> has no certificate for <bucket>.storage.<domain>.
        config=Config(s3={"addressing_style": "path"}),
    )


def _bucket() -> str:
    return os.environ.get("WHISK_STORAGE_BUCKET", "")


def _tree_key(tree: str, rel: str) -> str:
    return f"{os.environ.get('WHISK_STORAGE_PREFIX', '')}trees/{tree}/{rel}"


def _index_key(tree: str) -> str:
    return f"{os.environ.get('WHISK_STORAGE_PREFIX', '')}trees/{tree}.index.json"


def hash_dir(directory: str) -> dict[str, str]:
    """The index of the directory as it stands: every regular file's SHA-256, keyed by its path
    with forward slashes. A missing directory is an empty tree."""
    root = Path(directory)
    if not root.is_dir():
        return {}
    return {
        p.relative_to(root).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
        for p in root.rglob("*")
        if p.is_file() and not p.is_symlink()
    }


def _read_index(client, tree: str) -> dict[str, str]:
    try:
        body = client.get_object(Bucket=_bucket(), Key=_index_key(tree))["Body"].read()
    except ClientError as err:
        if err.response.get("Error", {}).get("Code") in ("NoSuchKey", "404"):
            return {}
        raise
    return json.loads(body)


def pull_tree(client, tree: str, directory: str) -> None:
    """Make the directory hold exactly the tree's files as last pushed: files that differ are
    downloaded, files the tree no longer has are removed. A tree never pushed leaves it empty."""
    remote, local = _read_index(client, tree), hash_dir(directory)
    for rel in sorted(remote):
        if local.get(rel) == remote[rel]:
            continue
        target = Path(directory, *rel.split("/"))
        target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
        client.download_file(_bucket(), _tree_key(tree, rel), str(target))
    for rel in local:
        if rel not in remote:
            Path(directory, *rel.split("/")).unlink(missing_ok=True)


def push_tree(client, tree: str, directory: str) -> None:
    """Make the tree in the bucket match the directory: changed files are uploaded, then the
    index, then files the directory no longer has are deleted. A push cut short leaves the
    previous index in place, so the next pull still reads a whole tree and the next push
    repeats the work."""
    remote, local = _read_index(client, tree), hash_dir(directory)
    for rel in sorted(local):
        if remote.get(rel) != local[rel]:
            client.upload_file(str(Path(directory, *rel.split("/"))), _bucket(), _tree_key(tree, rel))
    client.put_object(Bucket=_bucket(), Key=_index_key(tree), Body=json.dumps(local).encode(), ContentType="application/json")
    for rel in remote:
        if rel not in local:
            client.delete_object(Bucket=_bucket(), Key=_tree_key(tree, rel))

