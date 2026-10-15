package doctor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
)

// W010 runs the same rule set as the pre-receive hook: gitleaks' default configuration,
// extended by the repository's own .gitleaks.toml when it has one. Values are never returned,
// only the rule, file and line.

type secretHit struct {
	Rule string
	File string
	Line int
}

func newDetector(r Repo) (*detect.Detector, error) {
	v := viper.New()
	v.SetConfigType("toml")
	src := r.Read(".gitleaks.toml")
	if src == nil {
		src = []byte(config.DefaultConfig)
	}
	if err := v.ReadConfig(bytes.NewReader(src)); err != nil {
		return nil, fmt.Errorf(".gitleaks.toml: %v", err)
	}
	var vc config.ViperConfig
	if err := v.Unmarshal(&vc); err != nil {
		return nil, fmt.Errorf(".gitleaks.toml: %v", err)
	}
	cfg, err := vc.Translate()
	if err != nil {
		return nil, fmt.Errorf(".gitleaks.toml: %v", err)
	}
	d := detect.NewDetector(cfg)
	if r.Has(".gitleaksignore") {
		_ = d.AddGitleaksIgnore(r.Dir + "/.gitleaksignore")
	}
	return d, nil
}

func secretFindings(r Repo) ([]secretHit, error) {
	d, err := newDetector(r)
	if err != nil {
		return nil, err
	}
	var out []secretHit
	for _, f := range r.Files {
		if !f.Tracked || f.Content == nil || f.Path == ".gitleaks.toml" {
			continue
		}
		for _, finding := range d.Detect(detect.Fragment{Raw: string(f.Content), FilePath: f.Path}) {
			out = append(out, secretHit{Rule: finding.RuleID, File: f.Path, Line: finding.StartLine + 1})
		}
	}
	return out, nil
}

// envExampleClean reports whether a .env.example holds only names (NAME=) and comments.
func envExampleClean(content []byte) bool {
	for _, line := range strings.Split(string(content), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		_, value, ok := strings.Cut(t, "=")
		if !ok || strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}
