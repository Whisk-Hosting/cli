// Package mimetypes names a file's type from its name and says whether a type is worth
// compressing, from mime-db (https://github.com/jshttp/mime-db, MIT), the list most web servers
// and frameworks share: every type IANA registers and those Apache and nginx add. update.sh takes a
// newer release of the list.
package mimetypes

import (
	"path"
	"strings"
)

// ByName is the type of a file named name, from its extension, or "" for an extension no list
// names.
func ByName(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(name), "."))
	if ext == "" {
		return ""
	}
	return byExtension[ext]
}

// Compressible is whether gzip is worth its time on a type: what mime-db says when it says, else
// text, JSON, XML, JavaScript and 3D or CAD models, whose formats are text or sparse binary.
// Parameters (; charset=…) are ignored.
func Compressible(contentType string) bool {
	t, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(contentType)), ";")
	t = strings.TrimSpace(t)
	if t == "application/octet-stream" {
		return false // unknown bytes, which mime-db calls compressible
	}
	if c, ok := compressible[t]; ok {
		return c
	}
	family, sub, _ := strings.Cut(t, "/")
	switch {
	case family == "text", family == "model":
		return true
	case strings.HasSuffix(sub, "+json"), strings.HasSuffix(sub, "+xml"), strings.HasSuffix(sub, "+text"):
		return true
	case strings.Contains(sub, "javascript"):
		return true
	}
	return false
}

// Settle is the type to keep for a file named name that a client said is given: the name's type
// when the client said nothing or only something generic (application/octet-stream, text/plain),
// since a browser or a tool guessing says that of any file it does not know, and what the client
// said otherwise. A file no list or client names is application/octet-stream.
func Settle(name, given string) string {
	byName := ByName(name)
	base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(given)), ";")
	switch strings.TrimSpace(base) {
	case "", "application/octet-stream", "binary/octet-stream", "application/unknown", "text/plain":
		if byName != "" && byName != strings.TrimSpace(base) {
			return byName
		}
	}
	if strings.TrimSpace(given) == "" {
		return "application/octet-stream"
	}
	return given
}
