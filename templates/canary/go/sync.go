package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Working files for batch jobs (CONTRACT.md §8). The container's disk is /tmp, which is
// emptied on every restart, so a job that keeps a tree of files between runs keeps it in the
// app's bucket and works on a copy: PullTree before the run, PushTree during and after it.
// Each tree has an index of every file's SHA-256 beside it, so a pull downloads only what
// differs from /tmp and a push uploads only what changed. Keep one run per tree at a time with
// a lease row in the database. Needs `storage: true`.

// treeIndex maps each file's path inside the tree, with forward slashes, to its SHA-256.
type treeIndex map[string]string

var treeName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// storageClient is the bucket client, or nil when the manifest did not ask for storage.
func storageClient() (*minio.Client, error) {
	endpoint := os.Getenv("WHISK_STORAGE_ENDPOINT")
	if endpoint == "" {
		return nil, nil
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	return minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(os.Getenv("WHISK_STORAGE_ACCESS_KEY"), os.Getenv("WHISK_STORAGE_SECRET_KEY"), ""),
		Secure: u.Scheme == "https",
		Region: os.Getenv("WHISK_STORAGE_REGION"),
		// The bucket goes in the path: storage.<domain> has no certificate for <bucket>.storage.<domain>.
		BucketLookup: minio.BucketLookupPath,
	})
}

// validTreeFile reports whether a request names a tree and a file inside it: the file is a
// clean relative path that stays in the tree's directory.
func validTreeFile(tree, file string) bool {
	return treeName.MatchString(tree) && file != "." && path.Clean(file) == file && filepath.IsLocal(filepath.FromSlash(file)) && !strings.HasPrefix(file, "..")
}

// treePath is where a file named in a tree's index lands under dir. The index is read from the
// bucket, so a name that would climb out of dir is refused rather than written.
func treePath(dir, rel string) (string, error) {
	local := filepath.FromSlash(rel)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("the tree index names %q, which is outside the tree", rel)
	}
	return filepath.Join(dir, local), nil
}

// treeKey is where a file of the tree lives in the bucket, under the app's own prefix.
func treeKey(tree, rel string) string {
	return os.Getenv("WHISK_STORAGE_PREFIX") + "trees/" + tree + "/" + rel
}

func indexKey(tree string) string {
	return os.Getenv("WHISK_STORAGE_PREFIX") + "trees/" + tree + ".index.json"
}

// PullTree makes dir hold exactly the tree's files as last pushed: files that differ are
// downloaded, files the tree no longer has are removed. A tree never pushed leaves dir empty.
func PullTree(ctx context.Context, c *minio.Client, tree, dir string) error {
	remote, err := readIndex(ctx, c, tree)
	if err != nil {
		return err
	}
	local, err := hashDir(dir)
	if err != nil {
		return err
	}
	for _, rel := range sortedKeys(remote) {
		if local[rel] == remote[rel] {
			continue
		}
		target, err := treePath(dir, rel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := c.FGetObject(ctx, os.Getenv("WHISK_STORAGE_BUCKET"), treeKey(tree, rel), target, minio.GetObjectOptions{}); err != nil {
			return err
		}
	}
	for rel := range local {
		if _, kept := remote[rel]; !kept {
			if err := os.Remove(filepath.Join(dir, filepath.FromSlash(rel))); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// PushTree makes the tree in the bucket match dir: changed files are uploaded, then the index,
// then files dir no longer has are deleted. A push cut short leaves the previous index in
// place, so the next pull still reads a whole tree and the next push repeats the work.
func PushTree(ctx context.Context, c *minio.Client, tree, dir string) error {
	remote, err := readIndex(ctx, c, tree)
	if err != nil {
		return err
	}
	local, err := hashDir(dir)
	if err != nil {
		return err
	}
	bucket := os.Getenv("WHISK_STORAGE_BUCKET")
	for _, rel := range sortedKeys(local) {
		if remote[rel] == local[rel] {
			continue
		}
		if _, err := c.FPutObject(ctx, bucket, treeKey(tree, rel), filepath.Join(dir, filepath.FromSlash(rel)), minio.PutObjectOptions{}); err != nil {
			return err
		}
	}
	body, err := json.Marshal(local)
	if err != nil {
		return err
	}
	if _, err := c.PutObject(ctx, bucket, indexKey(tree), bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
		return err
	}
	for rel := range remote {
		if _, kept := local[rel]; !kept {
			if err := c.RemoveObject(ctx, bucket, treeKey(tree, rel), minio.RemoveObjectOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func readIndex(ctx context.Context, c *minio.Client, tree string) (treeIndex, error) {
	obj, err := c.GetObject(ctx, os.Getenv("WHISK_STORAGE_BUCKET"), indexKey(tree), minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()
	body, err := io.ReadAll(obj)
	if minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return treeIndex{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := treeIndex{}
	return out, json.Unmarshal(body, &out)
}

// hashDir is the index of dir as it stands: every regular file's SHA-256. A missing dir is
// an empty tree.
func hashDir(dir string) (treeIndex, error) {
	out := treeIndex{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == dir {
			return filepath.SkipDir
		}
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		sum, err := fileSHA256(p)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = sum
		return nil
	})
	return out, err
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sortedKeys(m treeIndex) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// syncRoute is the diagnostic the platform's harness drives: pull a tree into a fresh
// directory, optionally write one file and push, then pull again into another fresh directory
// and read the file back, which proves the bytes went through the bucket.
func syncRoute(w http.ResponseWriter, req *http.Request) {
	c, err := storageClient()
	if err != nil {
		writeJSON(w, 500, map[string]any{"configured": true, "error": err.Error()})
		return
	}
	if c == nil {
		writeJSON(w, 200, map[string]any{"configured": false})
		return
	}
	tree, file := req.URL.Query().Get("tree"), req.URL.Query().Get("file")
	if tree == "" {
		tree = "diag"
	}
	if file == "" {
		file = "diag.txt"
	}
	if !validTreeFile(tree, file) {
		writeJSON(w, 400, map[string]any{"configured": true, "error": "tree is lower case letters, digits and dashes; file is a relative path"})
		return
	}
	fail := func(err error) { writeJSON(w, 500, map[string]any{"configured": true, "error": err.Error()}) }
	ctx := req.Context()
	work, err := os.MkdirTemp("", "sync-")
	if err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(work)
	if err := PullTree(ctx, c, tree, work); err != nil {
		fail(err)
		return
	}
	if value := req.URL.Query().Get("value"); value != "" {
		target := filepath.Join(work, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			fail(err)
			return
		}
		if err := os.WriteFile(target, []byte(value), 0o600); err != nil {
			fail(err)
			return
		}
		if err := PushTree(ctx, c, tree, work); err != nil {
			fail(err)
			return
		}
	}
	fresh, err := os.MkdirTemp("", "sync-")
	if err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(fresh)
	if err := PullTree(ctx, c, tree, fresh); err != nil {
		fail(err)
		return
	}
	index, err := hashDir(fresh)
	if err != nil {
		fail(err)
		return
	}
	out := map[string]any{"configured": true, "tree": tree, "file": file, "files": len(index), "value": nil}
	if b, err := os.ReadFile(filepath.Join(fresh, filepath.FromSlash(file))); err == nil {
		out["value"] = string(b)
	}
	writeJSON(w, 200, out)
}
