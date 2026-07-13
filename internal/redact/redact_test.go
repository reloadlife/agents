package redact

import "testing"

func TestLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"hello", "hello"},
		{"token gho_abcdefghijklmnopqrstuv", "token [REDACTED]"},
		{"key sk-ant-api03-abcdefghijklmnop", "key [REDACTED]"},
	}
	for _, c := range cases {
		got := Line(c.in)
		if got != c.want {
			t.Errorf("Line(%q)=%q want %q", c.in, got, c.want)
		}
	}
}
