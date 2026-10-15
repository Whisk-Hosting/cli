package templates

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestArchiveURL(t *testing.T) {
	for _, c := range []struct{ version, want string }{
		{"2026.10.10-d6839a41", "https://github.com/whisk-hosting/shop/archive/refs/tags/2026.10.10-d6839a41.tar.gz"},
		{"dev", "https://github.com/whisk-hosting/shop/archive/refs/heads/main.tar.gz"},
		{"2026.10.10-d6839a41-dirty", "https://github.com/whisk-hosting/shop/archive/refs/heads/main.tar.gz"},
		{"", "https://github.com/whisk-hosting/shop/archive/refs/heads/main.tar.gz"},
	} {
		if got := ArchiveURL(RemoteBase+"/", "shop", c.version); got != c.want {
			t.Errorf("%q: got %s, want %s", c.version, got, c.want)
		}
	}
}

type entry struct {
	name string
	kind byte
	mode int64
	body string
}

func archive(t *testing.T, entries ...entry) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.kind, Mode: e.mode, Size: int64(len(e.body))}
		if e.kind == tar.TypeSymlink {
			h.Linkname, h.Size = "/etc/passwd", 0
		}
		if e.kind == tar.TypeDir {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	tw.Close()
	zw.Close()
	return &buf
}

func TestFromArchive(t *testing.T) {
	got, err := FromArchive(archive(t,
		entry{"shop-main/", tar.TypeDir, 0o755, ""},
		entry{"shop-main/whisk.yaml", tar.TypeReg, 0o644, "name: whisk-shop\n"},
		entry{"shop-main/scripts/run.sh", tar.TypeReg, 0o755, "#!/bin/sh\n"},
		entry{"shop-main/cms/src/app.ts", tar.TypeReg, 0o664, "export {}\n"},
	))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got["whisk.yaml"].Content != "name: whisk-shop\n" || got["scripts/run.sh"].Mode != 0o755 || got["cms/src/app.ts"].Mode != 0o644 {
		t.Errorf("got %+v", got)
	}
}

func TestFromArchiveRefuses(t *testing.T) {
	yaml := entry{"t/whisk.yaml", tar.TypeReg, 0o644, "name: x\n"}
	for _, c := range []struct {
		name string
		in   *bytes.Buffer
		want string
	}{
		{"a link", archive(t, yaml, entry{"t/evil", tar.TypeSymlink, 0o777, ""}), "not a plain file"},
		{"a path out of the folder", archive(t, yaml, entry{"t/../evil", tar.TypeReg, 0o644, "x"}), "will not do"},
		{"a second top folder", archive(t, yaml, entry{"other/x", tar.TypeReg, 0o644, "x"}), "outside its folder"},
		{"no manifest", archive(t, entry{"t/README.md", tar.TypeReg, 0o644, "x"}), "no whisk.yaml"},
		{"nothing", archive(t), "empty"},
		{"not gzip", bytes.NewBufferString("<html>not found</html>"), "not a gzip"},
	} {
		_, err := FromArchive(c.in)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
	}
}

func TestListNamesEveryTemplate(t *testing.T) {
	got := strings.Join(List(), ",")
	for _, n := range []string{"typescript", "python", "go", "shop", "site"} {
		if !strings.Contains(got, n) {
			t.Errorf("List() = %s lacks %s", got, n)
		}
	}
	if _, ok := Files("shop"); ok {
		t.Error("the shop is downloaded, not carried in the binary")
	}
}
