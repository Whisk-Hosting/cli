package manifest

import "testing"

func TestSetAlwaysOn(t *testing.T) {
	const base = "whisk: 1\nname: job-tracker\n"
	for _, tc := range []struct {
		name, src string
		on        bool
		want      string
	}{
		{"adds it when switched on", base, true, base + "always_on: true\n"},
		{"adds a newline first when the file lacks one", "whisk: 1\nname: job-tracker", true, "whisk: 1\nname: job-tracker\nalways_on: true\n"},
		{"leaves the file when switched off and absent", base, false, base},
		{"rewrites the line in place", "whisk: 1\nalways_on: false\nname: job-tracker\n", true, "whisk: 1\nalways_on: true\nname: job-tracker\n"},
		{"keeps the line's comment", base + "always_on: true # answers at once\n", false, base + "always_on: false # answers at once\n"},
		{"keeps a CRLF ending", "whisk: 1\r\nname: job-tracker\r\nalways_on: false\r\n", true, "whisk: 1\r\nname: job-tracker\r\nalways_on: true\r\n"},
		{"keeps the rest of the file", "# my app\nwhisk: 1\nname: job-tracker  # the name\nalways_on: false\nroutes:\n  public: [\"/\"]\n", true,
			"# my app\nwhisk: 1\nname: job-tracker  # the name\nalways_on: true\nroutes:\n  public: [\"/\"]\n"},
	} {
		got, err := SetAlwaysOn([]byte(tc.src), tc.on)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if string(got) != tc.want {
			t.Errorf("%s:\ngot  %q\nwant %q", tc.name, got, tc.want)
		}
	}
}

func TestSetAlwaysOnRefuses(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"not a manifest", "whisk: 1\n"},
		{"flow mapping", "{whisk: 1, name: job-tracker}\n"},
		{"not YAML", "whisk: [\n"},
	} {
		if _, err := SetAlwaysOn([]byte(tc.src), true); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}
