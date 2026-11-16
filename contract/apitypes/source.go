package apitypes

// SourceCommit is what a deploy from files did.
type SourceCommit struct {
	Commit      string `json:"commit"`
	Changed     bool   `json:"changed"`
	DeployID    string `json:"deploy_id,omitempty"`
	Environment string `json:"environment,omitempty"`
}

// SourceTreeEntry is one file of an app's code.
type SourceTreeEntry struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// SourceListing is what Read answers: the files under a folder, or one file's text.
type SourceListing struct {
	Commit    *string           `json:"commit"`
	Path      string            `json:"path,omitempty"`
	Files     []SourceTreeEntry `json:"files,omitempty"`
	Content   *string           `json:"content,omitempty"`
	Encoding  string            `json:"encoding,omitempty"`
	Bytes     int64             `json:"bytes,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

var _ = more.addWire(SourceCommit{}, SourceListing{}, SourceTreeEntry{})
