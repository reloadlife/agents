package redact

import "regexp"

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`gho_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`ghp_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`ghu_[A-Za-z0-9_]{20,}`),
	regexp.MustCompile(`sk-ant-[A-Za-z0-9\-_]{20,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`xai-[A-Za-z0-9]{20,}`),
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*\S+`),
}

// Line redacts known secret patterns from a log line.
func Line(s string) string {
	out := s
	for _, re := range patterns {
		out = re.ReplaceAllString(out, "[REDACTED]")
	}
	return out
}
