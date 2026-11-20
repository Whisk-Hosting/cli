package apitypes

// What the CLI tells Whisk when it is asked for something it does not have (CLI.md §7): the
// shape of the command it was given, never a value from it.

// CLIMissKind is how a command line failed before any command ran.
type CLIMissKind string

const (
	// MissUnknownCommand is a word where a command was expected that names none.
	MissUnknownCommand CLIMissKind = "unknown_command"
	// MissUnknownFlag is a flag the command does not have.
	MissUnknownFlag CLIMissKind = "unknown_flag"
	// MissMissingFlag is a required flag left out.
	MissMissingFlag CLIMissKind = "missing_flag"
	// MissFlagValue is a flag given without a value, or with one of the wrong type.
	MissFlagValue CLIMissKind = "flag_value"
	// MissFlagGroup is flags that must go together, or must not, given otherwise.
	MissFlagGroup CLIMissKind = "flag_group"
	// MissArgs is the wrong number or kind of arguments.
	MissArgs CLIMissKind = "args"
)

// CLIMissStatus is where an operator's decision leaves a shape of command line: open has none;
// resolved opens again when an agent reaches for it again; wont_fix stays closed.
type CLIMissStatus string

const (
	MissOpen     CLIMissStatus = "open"
	MissResolved CLIMissStatus = "resolved"
	MissWontFix  CLIMissStatus = "wont_fix"
)

// CLIMiss is POST /v1/cli/misses: one command line the CLI could not run. Command is the
// command path it reached, without "whisk" ("" for whisk itself); Word is the unknown command or
// the flags concerned, space-separated; Flags are the names of the flags given; Args is how many
// arguments were given. Agent names the coding agent running the CLI when it says so, and
// Terminal is whether a person's terminal ran it. No argument or flag value is ever sent.
type CLIMiss struct {
	Kind     CLIMissKind `json:"kind"`
	Command  string      `json:"command"`
	Word     string      `json:"word,omitempty"`
	Flags    []string    `json:"flags"`
	Args     int         `json:"args"`
	Version  string      `json:"version"`
	OS       string      `json:"os"`
	Arch     string      `json:"arch"`
	Agent    string      `json:"agent,omitempty"`
	Terminal bool        `json:"terminal"`
}

// CLIMissReceipt is the answer: whether the report was kept (an address past its hourly
// allowance is answered but not kept).
type CLIMissReceipt struct {
	Kept bool `json:"kept"`
}

var (
	_ = more.addWire(CLIMiss{}, CLIMissReceipt{})
	_ = more.addEnum(enumOf("CLIMissKind", "CLI_MISS_KINDS", MissUnknownCommand, MissUnknownFlag, MissMissingFlag, MissFlagValue, MissFlagGroup, MissArgs))
	_ = more.addEnum(enumOf("CLIMissStatus", "CLI_MISS_STATUSES", MissOpen, MissResolved, MissWontFix))
)
