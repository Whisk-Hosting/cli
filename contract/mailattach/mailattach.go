// Package mailattach is the rules for the files an app attaches to an email it sends through
// Whisk (CONTRACT.md §7 "Email", CONTROL-PLANE.md §6.12 "Attachments"): how many, how large, which
// types are refused, and how an inline image is tied to the HTML that shows it. The rules are
// pure and table-tested, and the platform and the local stub (whisk dev) both decide with them,
// so a message the stub takes is one the platform takes.
package mailattach

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/whisk-run/contract/apitypes"
)

// Limits on one message's attachments.
const (
	// MaxFiles is the most files one message may carry.
	MaxFiles = 10
	// MaxBytes is the most the files may hold together, decoded: 10 MB, which is about 13.4 MB
	// once base64 encoded on the wire. That is under every provider's limit (Resend's and SES's
	// 40 MB) and under what common mail servers accept (Gmail 25 MB, Outlook 20 MB, and many
	// company servers 10 MB of attachments), so a message that leaves Whisk also arrives.
	MaxBytes = 10 << 20
	// MaxRequestBytes is the most the JSON body of one send may be: the files base64 encoded,
	// the HTML and text bodies and the rest.
	MaxRequestBytes = 16 << 20
	// MaxFilename is the longest a file name may be, in bytes.
	MaxFilename = 255
	// MaxContentID is the longest a content_id may be.
	MaxContentID = 128
	// MaxStorageKey is the longest storage key, which is S3's own limit.
	MaxStorageKey = 1024
)

// Codes are the errors a send's attachments answer with (contract/errors.md).
const (
	CodeInvalid  = "EMAIL_ATTACHMENT_INVALID"
	CodeTooLarge = "EMAIL_ATTACHMENT_TOO_LARGE"
	CodeBlocked  = "EMAIL_ATTACHMENT_BLOCKED"
	CodeNotFound = "EMAIL_ATTACHMENT_NOT_FOUND"
)

// Request is one entry of a send body's attachments, as the app wrote it
// (apitypes.EmailAttachment). Exactly one of Content (the file, base64) and StorageKey (an
// object in the app's storage, its key in full, starting with WHISK_STORAGE_PREFIX) is given. A
// ContentID makes the file an inline image the HTML shows with <img src="cid:<content_id>">.
type Request = apitypes.EmailAttachment

// File is one attachment as it goes out. A file named by StorageKey has no Content until the
// caller reads it from storage.
type File struct {
	Filename    string
	ContentType string
	ContentID   string
	StorageKey  string
	Content     []byte
}

// Inline reports whether the file is an image the HTML shows rather than a file to open.
func (f File) Inline() bool { return f.ContentID != "" }

// Problem is why a message's attachments cannot be sent, in the platform's error codes.
type Problem struct {
	Code    string
	Message string
	Fix     string
	Details map[string]any
}

func (p *Problem) Error() string { return p.Code + ": " + p.Message }

func invalid(i int, message, fix string, details map[string]any) *Problem {
	d := map[string]any{"attachment": i}
	for k, v := range details {
		d[k] = v
	}
	return &Problem{Code: CodeInvalid, Message: message, Fix: fix, Details: d}
}

// contentIDPattern is what a content_id may hold: the characters of an RFC 5322 dot-atom with
// an optional @, so it is one token in a header and in a cid: URL alike.
var contentIDPattern = regexp.MustCompile(`^[A-Za-z0-9!#$%&'*+/=?^_{|}~.@-]+$`)

// cidReference is a cid: URL in HTML: what follows up to a quote, a bracket, a space or the
// closing parenthesis of a CSS url().
var cidReference = regexp.MustCompile(`(?i)cid:([^"'<>\s)]+)`)

// contentTypePattern is a MIME type with no parameters: type/subtype.
var contentTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]{0,63}/[a-z0-9][a-z0-9!#$&^_.+-]{0,126}$`)

// Plan checks the shape of every attachment and decodes the files sent as base64. html is the
// message's HTML body, which every inline image must be shown in and every cid: reference must
// name an inline image of; prefix is the app's storage prefix, which a storage_key must start
// with. The files a storage_key names come back without content, for the caller to read with
// at most Budget(files) bytes, then Check.
func Plan(in []Request, html, prefix string) ([]File, *Problem) {
	if len(in) == 0 {
		return nil, nil
	}
	if len(in) > MaxFiles {
		return nil, &Problem{Code: CodeInvalid,
			Message: fmt.Sprintf("A message may carry %d attachments; this one carries %d.", MaxFiles, len(in)),
			Fix:     "Send fewer files, or put them in one archive, or send a link to them instead.",
			Details: map[string]any{"attachments": len(in), "limit": MaxFiles}}
	}
	out := make([]File, 0, len(in))
	ids := map[string]int{}
	var total int64
	for i, r := range in {
		f, p := plan(i, r, prefix)
		if p != nil {
			return nil, p
		}
		if f.ContentID != "" {
			key := strings.ToLower(f.ContentID)
			if j, ok := ids[key]; ok {
				return nil, invalid(i, fmt.Sprintf("Attachments %d and %d both have content_id %q.", j, i, f.ContentID),
					"Give each inline image a content_id of its own.", map[string]any{"content_id": f.ContentID})
			}
			ids[key] = i
		}
		total += int64(len(f.Content))
		if total > MaxBytes {
			return nil, TooLarge(total)
		}
		out = append(out, f)
	}
	if p := references(out, html); p != nil {
		return nil, p
	}
	return out, nil
}

// plan is one attachment's checks.
func plan(i int, r Request, prefix string) (File, *Problem) {
	name, p := filename(i, r.Filename)
	if p != nil {
		return File{}, p
	}
	f := File{Filename: name}
	switch {
	case r.Content != "" && r.StorageKey != "":
		return File{}, invalid(i, fmt.Sprintf("Attachment %s has both content and storage_key.", name),
			"Send the file as content (base64) or name it in storage with storage_key, not both.", map[string]any{"filename": name})
	case r.Content == "" && r.StorageKey == "":
		return File{}, invalid(i, fmt.Sprintf("Attachment %s has no content.", name),
			"Send the file base64 encoded as content, or name an object in the app's storage with storage_key.", map[string]any{"filename": name})
	case r.Content != "":
		raw, ok := decode(r.Content)
		if !ok {
			return File{}, invalid(i, fmt.Sprintf("The content of attachment %s is not base64.", name),
				"Send the file's bytes base64 encoded (standard alphabet, as btoa, base64.b64encode and base64.StdEncoding write it).", map[string]any{"filename": name})
		}
		if len(raw) == 0 {
			return File{}, invalid(i, fmt.Sprintf("Attachment %s is empty.", name), "Leave out a file with nothing in it.", map[string]any{"filename": name})
		}
		f.Content = raw
	default:
		key, p := storageKey(i, name, r.StorageKey, prefix)
		if p != nil {
			return File{}, p
		}
		f.StorageKey = key
	}
	ct, p := contentType(i, name, r.ContentType)
	if p != nil {
		return File{}, p
	}
	f.ContentType = ct
	if r.ContentID != "" {
		id := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(r.ContentID), "<"), ">")
		if len(id) > MaxContentID || !contentIDPattern.MatchString(id) {
			return File{}, invalid(i, fmt.Sprintf("Attachment %s has a content_id that cannot be used: %q.", name, r.ContentID),
				fmt.Sprintf("Use a short id of letters, digits, dots, hyphens and @, up to %d characters, such as logo or logo@acme.", MaxContentID),
				map[string]any{"filename": name})
		}
		if !strings.HasPrefix(ct, "image/") {
			return File{}, invalid(i, fmt.Sprintf("Attachment %s has a content_id but is %s, not an image.", name, ct),
				"Give content_id only to images the HTML shows; attach other files without one.", map[string]any{"filename": name, "content_type": ct})
		}
		f.ContentID = id
	}
	if p := blockedName(i, name, ct); p != nil {
		return File{}, p
	}
	return f, nil
}

// filename is the name as it appears in the mail, and whether it is one: a plain name with no
// directory, no control characters, valid UTF-8 and at most MaxFilename bytes.
func filename(i int, raw string) (string, *Problem) {
	name := strings.TrimSpace(raw)
	fix := fmt.Sprintf("Send filename as the file's own name with its extension, such as report.pdf, at most %d bytes.", MaxFilename)
	switch {
	case name == "":
		return "", invalid(i, fmt.Sprintf("Attachment %d has no filename.", i), fix, nil)
	case len(name) > MaxFilename, !utf8.ValidString(name), strings.ContainsAny(name, `/\`),
		name == ".", name == "..", strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "", invalid(i, fmt.Sprintf("Attachment %d has a filename that cannot be used: %q.", i, raw), fix, nil)
	}
	return name, nil
}

// decode reads standard base64, with or without padding, ignoring the line breaks and spaces a
// wrapping encoder adds.
func decode(s string) ([]byte, bool) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
		return raw, true
	}
	raw, err := base64.RawStdEncoding.DecodeString(s)
	return raw, err == nil
}

// storageKey is the object a storage_key names, which has to be under the app's own prefix.
func storageKey(i int, name, key, prefix string) (string, *Problem) {
	fix := "Send storage_key as the object's full key, the app's WHISK_STORAGE_PREFIX followed by its name, such as " + prefix + "reports/october.pdf."
	bad := len(key) > MaxStorageKey || !utf8.ValidString(key) || strings.IndexFunc(key, unicode.IsControl) >= 0 ||
		strings.HasPrefix(key, "/") || strings.HasSuffix(key, "/")
	for _, seg := range strings.Split(key, "/") {
		if seg == "." || seg == ".." {
			bad = true
		}
	}
	if bad {
		return "", invalid(i, fmt.Sprintf("Attachment %s has a storage_key that cannot be used.", name), fix, map[string]any{"filename": name})
	}
	if prefix == "" || !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
		return "", &Problem{Code: CodeNotFound,
			Message: fmt.Sprintf("Attachment %s names %s, which is not in this app's storage.", name, key),
			Fix:     fix + " An app attaches only its own files.",
			Details: map[string]any{"attachment": i, "filename": name, "storage_key": key}}
	}
	return key, nil
}

// types is the content type a file name's extension implies, for a file sent without one. It
// is a table rather than the system's, so every machine decides alike.
var types = map[string]string{
	".pdf": "application/pdf", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".svg": "image/svg+xml", ".avif": "image/avif",
	".heic": "image/heic", ".bmp": "image/bmp", ".ico": "image/x-icon", ".tif": "image/tiff", ".tiff": "image/tiff",
	".txt": "text/plain", ".csv": "text/csv", ".ics": "text/calendar", ".vcf": "text/vcard",
	".htm": "text/html", ".html": "text/html", ".md": "text/markdown", ".json": "application/json",
	".xml": "application/xml", ".zip": "application/zip", ".gz": "application/gzip",
	".doc": "application/msword", ".xls": "application/vnd.ms-excel", ".ppt": "application/vnd.ms-powerpoint",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".odt":  "application/vnd.oasis.opendocument.text", ".ods": "application/vnd.oasis.opendocument.spreadsheet",
	".rtf": "application/rtf", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".wav": "audio/wav",
	".mp4": "video/mp4", ".mov": "video/quicktime", ".eml": "message/rfc822",
}

// ContentTypeFor is the content type a file name's extension implies, application/octet-stream
// when the table has none.
func ContentTypeFor(name string) string {
	if t, ok := types[strings.ToLower(path.Ext(name))]; ok {
		return t
	}
	return "application/octet-stream"
}

func contentType(i int, name, raw string) (string, *Problem) {
	ct := strings.ToLower(strings.TrimSpace(raw))
	if before, _, ok := strings.Cut(ct, ";"); ok {
		ct = strings.TrimSpace(before) // parameters such as charset are the transport's to write
	}
	if ct == "" {
		return ContentTypeFor(name), nil
	}
	if !contentTypePattern.MatchString(ct) {
		return "", invalid(i, fmt.Sprintf("Attachment %s has a content_type that is not a MIME type: %q.", name, raw),
			"Send content_type as type/subtype, such as application/pdf or image/png, or leave it out to have it follow the extension.",
			map[string]any{"filename": name})
	}
	return ct, nil
}

// Blocked are the extensions no message may carry, whatever the content type says: programs,
// scripts, shortcuts, installers and disk images, which mail is the commonest way to deliver
// malware in. The list is the one Gmail refuses, which most mail servers follow, so a message
// carrying one would be refused at the other end anyway.
var Blocked = []string{
	".ade", ".adp", ".apk", ".appx", ".appxbundle", ".bat", ".cab", ".chm", ".cmd", ".com", ".cpl",
	".diagcab", ".diagcfg", ".diagpack", ".dll", ".dmg", ".ex", ".ex_", ".exe", ".hta", ".img", ".ins",
	".iso", ".isp", ".jar", ".jnlp", ".js", ".jse", ".lib", ".lnk", ".mde", ".mjs", ".msc", ".msi",
	".msix", ".msixbundle", ".msp", ".mst", ".nsh", ".pif", ".ps1", ".scr", ".sct", ".shb", ".sys",
	".vb", ".vbe", ".vbs", ".vhd", ".vxd", ".wsc", ".wsf", ".wsh", ".xll",
}

// BlockedTypes are the content types of programs, refused whatever the file is called.
var BlockedTypes = []string{
	"application/x-msdownload", "application/x-msdos-program", "application/x-dosexec",
	"application/x-executable", "application/x-ms-installer", "application/x-msi",
	"application/vnd.microsoft.portable-executable", "application/java-archive",
	"application/x-ms-shortcut", "application/hta", "application/x-apple-diskimage",
	"application/vnd.android.package-archive",
}

var blocked = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range Blocked {
		m[e] = true
	}
	for _, t := range BlockedTypes {
		m[t] = true
	}
	return m
}()

// blockedExt reports whether a file name ends in a blocked extension, ignoring the trailing
// dots and spaces Windows drops when it opens a file.
func blockedExt(name string) (string, bool) {
	ext := strings.ToLower(path.Ext(strings.TrimRight(name, ". ")))
	return ext, ext != "" && blocked[ext]
}

func blockedName(i int, name, ct string) *Problem {
	if ext, ok := blockedExt(name); ok {
		return refused(i, name, "its type, "+ext+", is a program or script mail servers refuse", map[string]any{"extension": ext})
	}
	if blocked[ct] {
		return refused(i, name, "its content type, "+ct+", is a program mail servers refuse", map[string]any{"content_type": ct})
	}
	return nil
}

func refused(i int, name, why string, details map[string]any) *Problem {
	d := map[string]any{"attachment": i, "filename": name}
	for k, v := range details {
		d[k] = v
	}
	return &Problem{Code: CodeBlocked,
		Message: fmt.Sprintf("Attachment %s cannot be sent: %s.", name, why),
		Fix:     "Send documents, images and data files, not programs. To share a program, put it in storage and send a link to it.",
		Details: d}
}

// references ties inline images to the HTML: every content_id is shown by a cid: URL in the
// HTML, and every cid: URL in the HTML names one. An inline image nobody shows lands as a
// loose attachment, and a cid: with no image is a broken picture.
func references(files []File, html string) *Problem {
	shown := map[string]bool{}
	for _, m := range cidReference.FindAllStringSubmatch(html, -1) {
		shown[strings.ToLower(m[1])] = true
	}
	given := map[string]bool{}
	for i, f := range files {
		if !f.Inline() {
			continue
		}
		given[strings.ToLower(f.ContentID)] = true
		if html == "" {
			return invalid(i, fmt.Sprintf("Attachment %s is an inline image, and the message has no HTML to show it in.", f.Filename),
				`Send html with <img src="cid:`+f.ContentID+`">, or leave content_id out to attach the file.`,
				map[string]any{"filename": f.Filename, "content_id": f.ContentID})
		}
		if !shown[strings.ToLower(f.ContentID)] {
			return invalid(i, fmt.Sprintf("Attachment %s has content_id %s, and the HTML never shows cid:%s.", f.Filename, f.ContentID, f.ContentID),
				`Show it with <img src="cid:`+f.ContentID+`">, or leave content_id out to attach the file.`,
				map[string]any{"filename": f.Filename, "content_id": f.ContentID})
		}
	}
	missing := []string{}
	for id := range shown {
		if !given[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return &Problem{Code: CodeInvalid,
			Message: fmt.Sprintf("The HTML shows cid:%s, and no attachment has that content_id.", missing[0]),
			Fix:     "Attach the image with content_id set to the name after cid:, or change the HTML.",
			Details: map[string]any{"content_ids": missing}}
	}
	return nil
}

// Total is the bytes the files hold now.
func Total(files []File) int64 {
	var n int64
	for _, f := range files {
		n += int64(len(f.Content))
	}
	return n
}

// Budget is how much more the files named in storage may hold together.
func Budget(files []File) int64 { return MaxBytes - Total(files) }

// TooLarge is the problem for files that hold total bytes together.
func TooLarge(total int64) *Problem {
	return &Problem{Code: CodeTooLarge,
		Message: fmt.Sprintf("The attachments hold %s together; a message may carry %s.", size(total), size(MaxBytes)),
		Fix:     "Send smaller files, fewer of them, or a link to the file in storage instead of the file.",
		Details: map[string]any{"bytes": total, "limit_bytes": MaxBytes}}
}

// Check is the last word once every file's content is in hand: none is empty, together they fit,
// and none is a program whatever its name says. A ZIP archive is opened far enough to read its
// names, and one holding a blocked file is refused as that file would be.
func Check(files []File) *Problem {
	for i, f := range files {
		if f.Content == nil && f.StorageKey != "" {
			return invalid(i, fmt.Sprintf("Attachment %s was not read from storage.", f.Filename), "Send it again.", map[string]any{"filename": f.Filename})
		}
		if len(f.Content) == 0 {
			return invalid(i, fmt.Sprintf("Attachment %s is empty.", f.Filename), "Leave out a file with nothing in it.", map[string]any{"filename": f.Filename})
		}
	}
	if total := Total(files); total > MaxBytes {
		return TooLarge(total)
	}
	for i, f := range files {
		if program(f.Content) {
			return refused(i, f.Filename, "it is a program", map[string]any{"detected": "executable"})
		}
		if inner, ok := archived(f.Content); ok {
			return refused(i, f.Filename, "it is an archive holding "+inner+", a program or script mail servers refuse",
				map[string]any{"archived": inner})
		}
	}
	return nil
}

// program recognises the executables of Windows, Linux and macOS by their first bytes.
func program(b []byte) bool {
	switch {
	case len(b) >= 64 && b[0] == 'M' && b[1] == 'Z':
		return true
	case bytes.HasPrefix(b, []byte("\x7fELF")):
		return true
	case len(b) >= 4:
		for _, magic := range [][]byte{{0xfe, 0xed, 0xfa, 0xce}, {0xfe, 0xed, 0xfa, 0xcf}, {0xce, 0xfa, 0xed, 0xfe}, {0xcf, 0xfa, 0xed, 0xfe}} {
			if bytes.HasPrefix(b, magic) {
				return true
			}
		}
	}
	return false
}

// archived is the first blocked file a ZIP archive names. Office documents are ZIP archives
// too and hold nothing blocked; an archive that cannot be read is left to the receiving server.
func archived(b []byte) (string, bool) {
	if !bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		return "", false
	}
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return "", false
	}
	for _, f := range z.File {
		if _, ok := blockedExt(f.Name); ok {
			return f.Name, true
		}
	}
	return "", false
}

func size(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%d KB", (n+1023)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
