package workspaces

import "testing"

func TestNormalizeGitURL(t *testing.T) {
	cases := []struct {
		in, url, name string
		ok            bool
	}{
		{"owner/repo", "https://github.com/owner/repo.git", "repo", true},
		{"https://github.com/o/r.git", "https://github.com/o/r.git", "r", true},
		{"git@github.com:o/r.git", "git@github.com:o/r.git", "r", true},
		{"github.com/o/r", "https://github.com/o/r", "r", true},
		{"", "", "", false},
		{"not a url", "", "", false},
		{"../evil", "", "", false},
	}
	for _, c := range cases {
		u, n, err := normalizeGitURL(c.in)
		if c.ok {
			if err != nil {
				t.Fatalf("%q: %v", c.in, err)
			}
			if u != c.url || n != c.name {
				t.Fatalf("%q => url=%q name=%q want %q %q", c.in, u, n, c.url, c.name)
			}
		} else if err == nil {
			t.Fatalf("%q: expected error", c.in)
		}
	}
}
