package apitypes

// SourceRequest is a deploy from files.
type SourceRequest struct {
	Files      []SourceFile `json:"files"`
	Delete     []string     `json:"delete"`
	Message    string       `json:"message"`
	BaseCommit string       `json:"base_commit"`
}

var _ = more.addWire(SourceRequest{})
