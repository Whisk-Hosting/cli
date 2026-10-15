package whisk

import (
	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// sourceBuild is false in the public CLI, which whisk update replaces (CLI.md §2).
var sourceBuild = false

// privateCmds is empty in the public CLI (CLI.md §5.14).
func privateCmds(*session) []*cobra.Command { return nil }

// privateFeedbackCmds adds nothing to whisk feedback in the public CLI (CLI.md §5.14).
func privateFeedbackCmds(*session, *cobra.Command, *api.FeedbackQuery) []*cobra.Command {
	return nil
}
