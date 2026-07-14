package ghauth

import (
	"strings"
	"testing"
)

func TestParseStatus(t *testing.T) {
	raw := `github.com
  ✓ Logged in to github.com account reloadlife (/root/.config/gh/hosts.yml)
  - Active account: true
  - Git operations protocol: https
  - Token: gho_XXXXXXXX
  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'

  ✓ Logged in to github.com account mamaru-a (/root/.config/gh/hosts.yml)
  - Active account: false
  - Git operations protocol: https
  - Token: gho_YYYYYYYY
  - Token scopes: 'gist', 'read:org', 'repo'
`
	acc := parseStatus(raw)
	if len(acc) != 2 {
		t.Fatalf("got %d accounts: %+v", len(acc), acc)
	}
	if acc[0].User != "reloadlife" || !acc[0].Active {
		t.Fatalf("first: %+v", acc[0])
	}
	if acc[1].User != "mamaru-a" || acc[1].Active {
		t.Fatalf("second: %+v", acc[1])
	}
	if acc[0].GitProtocol != "https" {
		t.Fatalf("proto %q", acc[0].GitProtocol)
	}
	if len(acc[0].Scopes) != 4 {
		t.Fatalf("scopes %+v", acc[0].Scopes)
	}
}

func TestRedactTokens(t *testing.T) {
	in := "Token: gho_abc123DEF and ghp_zzz999"
	out := redactTokens(in)
	if strings.Contains(out, "gho_") || strings.Contains(out, "ghp_") {
		t.Fatalf("not redacted: %q", out)
	}
}
