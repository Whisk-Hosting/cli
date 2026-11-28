package whisk

import (
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

func TestFilesRestoredLineSaysWhatTheRestoreDid(t *testing.T) {
	cases := []struct {
		in   api.FilesRestored
		want string
	}{
		{api.FilesRestored{}, "No deleted file to put back since then."},
		{api.FilesRestored{Restored: 1}, "Put back 1 deleted file."},
		{api.FilesRestored{Restored: 12, Newer: 2}, "Put back 12 deleted files. Left 2 kept: a newer file has that name now."},
		{api.FilesRestored{Restored: 2000, More: true}, "Put back 2000 deleted files. More remain: run it again to carry on."},
	}
	for _, tc := range cases {
		if got := filesRestoredLine(tc.in); got != tc.want {
			t.Errorf("filesRestoredLine(%+v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
