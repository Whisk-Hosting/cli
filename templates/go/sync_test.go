package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

// TestTreeRoundTrip pushes a tree, changes it, and pulls it into a fresh directory. It needs
// an S3 store: set WHISK_STORAGE_ENDPOINT, _BUCKET, _ACCESS_KEY, _SECRET_KEY and _PREFIX, as
// `whisk dev` does with storage: true.
func TestTreeRoundTrip(t *testing.T) {
	c, err := storageClient()
	if err != nil {
		t.Fatal(err)
	}
	if c == nil {
		t.Skip("WHISK_STORAGE_ENDPOINT is not set")
	}
	ctx := context.Background()
	bucket := os.Getenv("WHISK_STORAGE_BUCKET")
	if ok, _ := c.BucketExists(ctx, bucket); !ok {
		if err := c.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	tree := fmt.Sprintf("test-%d", time.Now().UnixNano())
	write := func(dir, rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	empty := t.TempDir()
	if err := PullTree(ctx, c, tree, empty); err != nil {
		t.Fatalf("pull of a tree never pushed: %v", err)
	}
	if idx, _ := hashDir(empty); len(idx) != 0 {
		t.Fatalf("a tree never pushed pulled %v", idx)
	}

	work := t.TempDir()
	write(work, "a.txt", "one")
	write(work, "deep/b.txt", "two")
	write(work, "gone.txt", "three")
	if err := PushTree(ctx, c, tree, work); err != nil {
		t.Fatal(err)
	}
	write(work, "a.txt", "one, changed")
	if err := os.Remove(filepath.Join(work, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	if err := PushTree(ctx, c, tree, work); err != nil {
		t.Fatal(err)
	}

	// A fresh directory that already holds a stale file and one the tree never had.
	fresh := t.TempDir()
	write(fresh, "a.txt", "stale")
	write(fresh, "extra.txt", "not in the tree")
	if err := PullTree(ctx, c, tree, fresh); err != nil {
		t.Fatal(err)
	}
	got, err := hashDir(fresh)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hashDir(work)
	if len(got) != len(want) {
		t.Fatalf("pulled %v, want %v", got, want)
	}
	for rel, sum := range want {
		if got[rel] != sum {
			t.Errorf("%s: pulled %s, want %s", rel, got[rel], sum)
		}
	}
	if _, err := c.StatObject(ctx, bucket, treeKey(tree, "gone.txt"), minio.StatObjectOptions{}); err == nil {
		t.Error("a file removed from the tree is still in the bucket")
	}
}

// TestHashDir checks the local index: regular files only, forward-slash paths, and a missing
// directory is an empty tree.
func TestHashDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "x", "y"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "y", "f"), []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := hashDir(dir)
	if err != nil || len(idx) != 1 || idx["x/y/f"] != "8f434346648f6b96df89dda901c5176b10a6d83961dd3c1ac88b59b2dc327aa4" {
		t.Errorf("hashDir = %v, %v", idx, err)
	}
	missing, err := hashDir(filepath.Join(dir, "nothing"))
	if err != nil || len(missing) != 0 {
		t.Errorf("a missing directory = %v, %v; want an empty tree", missing, err)
	}
}
