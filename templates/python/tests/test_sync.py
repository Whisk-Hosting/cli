"""The working-tree helper (app/sync.py). The round trip needs an S3 store: set the
WHISK_STORAGE_* variables as `whisk dev` does with storage: true."""

import os
import time

import pytest

from app.sync import hash_dir, pull_tree, push_tree, storage_client


def test_hash_dir(tmp_path):
    (tmp_path / "x" / "y").mkdir(parents=True)
    (tmp_path / "x" / "y" / "f").write_text("hi")
    assert hash_dir(str(tmp_path)) == {"x/y/f": "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4"}
    assert hash_dir(str(tmp_path / "nothing")) == {}


@pytest.mark.skipif(not os.environ.get("WHISK_STORAGE_ENDPOINT"), reason="WHISK_STORAGE_ENDPOINT is not set")
def test_tree_round_trip(tmp_path):
    client = storage_client()
    tree = f"test-{time.time_ns()}"
    work, fresh = tmp_path / "work", tmp_path / "fresh"
    (work / "deep").mkdir(parents=True)
    (work / "a.txt").write_text("one")
    (work / "deep" / "b.txt").write_text("two")
    (work / "gone.txt").write_text("three")
    push_tree(client, tree, str(work))
    (work / "a.txt").write_text("one, changed")
    (work / "gone.txt").unlink()
    push_tree(client, tree, str(work))
    fresh.mkdir()
    (fresh / "a.txt").write_text("stale")
    (fresh / "extra.txt").write_text("not in the tree")
    pull_tree(client, tree, str(fresh))
    assert hash_dir(str(fresh)) == hash_dir(str(work))
