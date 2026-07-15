package workspaces

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/reloadlife/agents/internal/config"
	"github.com/reloadlife/agents/internal/pathallow"
)

// CreateRequest creates a new directory under the workspace root.
type CreateRequest struct {
	// Name is a relative path (e.g. "my-app" or "clients/acme").
	Name string `json:"name"`
	// Path is an alias for Name (some clients prefer "path").
	Path string `json:"path,omitempty"`
}

var safeSegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Create makes a new directory under workspace_root if allowed by the path allowlist.
// It is idempotent: if the directory already exists, it returns success.
func Create(cfg *config.Config, req CreateRequest) (*Entry, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(req.Path)
	}
	name = filepath.ToSlash(name)
	name = strings.Trim(name, "/")
	name = strings.TrimPrefix(name, "./")
	if name == "" || name == "." {
		return nil, fmt.Errorf("name is required (relative folder under workspace root)")
	}
	if filepath.IsAbs(name) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("name must be a relative path without ..")
	}
	// Validate each path segment (prevent empty // and weird names).
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return nil, fmt.Errorf("invalid path segment in %q", name)
		}
		if strings.HasPrefix(seg, ".") {
			return nil, fmt.Errorf("hidden path segments not allowed (%q)", seg)
		}
		if !safeSegment.MatchString(seg) {
			return nil, fmt.Errorf("invalid path segment %q (use letters, numbers, . _ -)", seg)
		}
	}

	patterns := cfg.Allow.Paths
	abs, err := pathallow.Resolve(cfg.WorkspaceRoot, name, patterns)
	if err != nil {
		return nil, err
	}
	if st, err := os.Stat(abs); err == nil {
		if !st.IsDir() {
			return nil, fmt.Errorf("%q exists and is not a directory", name)
		}
		// already exists — ok
	} else {
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir: %w", err)
		}
	}

	rel := name
	return &Entry{
		Path:   rel,
		Abs:    abs,
		Exists: true,
		IsDir:  true,
	}, nil
}
