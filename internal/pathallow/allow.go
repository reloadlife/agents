package pathallow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Resolve checks that rel (repo/cwd request) maps under workspaceRoot and matches allow patterns.
// Returns absolute path.
func Resolve(workspaceRoot, rel string, patterns []string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		rel = "."
	}
	// forbid absolute escape attempts from client
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("cwd must be relative to workspace")
	}
	if strings.Contains(rel, "..") {
		return "", fmt.Errorf("cwd must not contain ..")
	}

	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	root = filepath.Clean(root)

	joined := filepath.Join(root, filepath.Clean(rel))
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	// resolve symlinks when the path exists; otherwise keep abs under root
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	abs = filepath.Clean(abs)

	if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
		return "", fmt.Errorf("cwd escapes workspace root")
	}

	relToRoot, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	relToRoot = filepath.ToSlash(relToRoot)
	if !matchAny(relToRoot, patterns) {
		return "", fmt.Errorf("cwd %q not in allowlist (allowed: %s)", relToRoot, formatPatterns(patterns))
	}
	return abs, nil
}

func formatPatterns(patterns []string) string {
	if len(patterns) == 0 {
		return "(any under workspace)"
	}
	return strings.Join(patterns, ", ")
}

func matchAny(rel string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if match(rel, p) {
			return true
		}
	}
	return false
}

func match(rel, pattern string) bool {
	pattern = strings.TrimSpace(filepath.ToSlash(pattern))
	if pattern == "" {
		return false
	}
	if pattern == "." || pattern == "*" || pattern == "**" {
		return true
	}
	// prefix/**
	if strings.HasSuffix(pattern, "/**") {
		prefix := strings.TrimSuffix(pattern, "/**")
		return rel == prefix || strings.HasPrefix(rel, prefix+"/")
	}
	// one segment wildcard: foo/*
	if strings.HasSuffix(pattern, "/*") {
		prefix := strings.TrimSuffix(pattern, "/*")
		if !strings.HasPrefix(rel, prefix+"/") {
			return false
		}
		rest := strings.TrimPrefix(rel, prefix+"/")
		return rest != "" && !strings.Contains(rest, "/")
	}
	// exact or prefix directory
	if rel == pattern {
		return true
	}
	if strings.HasPrefix(rel, pattern+"/") {
		return true
	}
	// simple glob
	ok, _ := filepath.Match(pattern, rel)
	return ok
}
