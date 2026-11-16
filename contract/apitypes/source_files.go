package apitypes

// SourceFile is one file to write: its path and its content, as text or base64.
type SourceFile struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"` // utf8 (default) | base64
}

var _ = more.addWire(SourceFile{})
