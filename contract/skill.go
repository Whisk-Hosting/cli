package contract

import (
	"embed"
	"regexp"
	"strings"
	"sync"
)

//go:embed skill/*.md
var skillFiles embed.FS

// SkillFile is one file holding the rest of a section of SKILL.md (CONTRACT.md §12): SKILL.md
// keeps what every app needs, and a section an app needs only sometimes starts there and ends
// with a line naming its file.
type SkillFile struct {
	Name string // the file name, as skill.<domain> serves it
	Text string
}

// skillPointer is the line that ends a section in SKILL.md and names the file with the rest.
var skillPointer = regexp.MustCompile("(?m)^\\*\\*More in \\[`([a-z0-9-]+\\.md)`\\]\\(https://skill\\.whisk\\.run/([a-z0-9-]+\\.md)\\):\\*\\*.*$")

// SkillFiles answers the skill's files in the order SKILL.md names them. A file SKILL.md does
// not name is left out, and a name with no file is answered empty; the contract's tests refuse
// both.
var SkillFiles = sync.OnceValue(func() []SkillFile {
	out := []SkillFile{}
	for _, m := range skillPointer.FindAllStringSubmatch(Skill, -1) {
		body, _ := skillFiles.ReadFile("skill/" + m[1])
		out = append(out, SkillFile{Name: m[1], Text: string(body)})
	}
	return out
})

// SkillFileNamed answers one of the skill's files by its name.
func SkillFileNamed(name string) (string, bool) {
	for _, f := range SkillFiles() {
		if f.Name == name {
			return f.Text, f.Text != ""
		}
	}
	return "", false
}

// skillFileBody is a file without its own title and opening line: the text that continues its
// section. Each file starts with a title, a blank line, one line saying what it is and a blank
// line. Pure.
func skillFileBody(text string) string {
	parts := strings.SplitN(text, "\n", 5)
	if len(parts) < 5 {
		return ""
	}
	return strings.TrimRight(parts[4], "\n")
}

// FullSkill is SKILL.md with each section's file put back where its line names it: the whole
// skill as one document, for a reader that takes everything at once. Pure.
func FullSkill() string {
	return skillPointer.ReplaceAllStringFunc(Skill, func(line string) string {
		text, _ := SkillFileNamed(skillPointer.FindStringSubmatch(line)[1])
		return skillFileBody(text)
	})
}
