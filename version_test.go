package main

import "testing"

// The linker stamps `version` and `commit` in at build time, so nothing here
// can check the stamping itself - that lives in the build scripts. What it can
// check is the part that would silently produce nonsense if the defaults were
// ever changed: an unstamped build must not render as "vdev", and a stamped
// one must show the commit, because that hash is the only thing that tells two
// downloads of the same rolling release apart.
func TestVersionString(t *testing.T) {
	origVersion, origCommit := version, commit
	t.Cleanup(func() { version, commit = origVersion, origCommit })

	cases := []struct {
		name    string
		version string
		commit  string
		want    string
	}{
		{"unstamped", "dev", "", "dev"},
		{"released", "1.1.0", "73ec59c", "v1.1.0 (73ec59c)"},
		{"no commit available", "1.1.0", "", "v1.1.0"},
		{"local build off a checkout", "dev", "73ec59c", "dev (73ec59c)"},
	}

	app := &App{}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version, commit = c.version, c.commit
			if got := app.Version(); got != c.want {
				t.Errorf("Version() = %q, want %q", got, c.want)
			}
		})
	}
}
