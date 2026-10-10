package connect

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
)

// A managed app's variant may write {setting.NAME} anywhere in its connections (MANAGED-APPS.md
// §2): the platform fills in the copy's setting before the connection is checked, granted or
// used, so the broker only ever sees plain text.
var (
	settingRef = regexp.MustCompile(`\{setting\.([A-Z][A-Z0-9_]{1,63})\}`)
	// SettingValue is what a setting a connection reads may hold: a host, a path, an ID or a
	// name, never a brace, quote or space that could change the recipe around it.
	SettingValue = regexp.MustCompile(`^[A-Za-z0-9._~@:/-]{1,253}$`)
)

// SettingPlaceholder stands in for every setting when a variant's connection is checked before
// any copy exists.
const SettingPlaceholder = "setting"

// Settings names the settings c reads, sorted, each once. Pure.
func Settings(c Connection) []string {
	raw, _ := json.Marshal(c)
	var out []string
	for _, m := range settingRef.FindAllStringSubmatch(string(raw), -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	slices.Sort(out)
	return out
}

// FillSettings is c with each {setting.NAME} replaced by values[NAME]. A setting it reads that
// has no value, or a value SettingValue refuses, is an error naming the setting. Pure.
func FillSettings(c Connection, values map[string]string) (Connection, error) {
	for _, name := range Settings(c) {
		v, ok := values[name]
		if !ok || v == "" {
			return c, fmt.Errorf("%s is not set, and the connection needs it", name)
		}
		if !SettingValue.MatchString(v) {
			return c, fmt.Errorf("%s may hold only letters, digits and . _ ~ @ : / -, up to 253", name)
		}
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return c, err
	}
	filled := settingRef.ReplaceAllStringFunc(string(raw), func(m string) string {
		return values[settingRef.FindStringSubmatch(m)[1]]
	})
	var out Connection
	if err := json.Unmarshal([]byte(filled), &out); err != nil {
		return c, err
	}
	return out, nil
}
