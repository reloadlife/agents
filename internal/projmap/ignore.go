package projmap

import (
	"path/filepath"
	"strings"
)

// Default ignore directory names (exact match on any path segment).
var ignoreDirNames = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	"node_modules": true, "vendor": true, "target": true,
	"dist": true, "build": true, "out": true, "bin": true,
	".next": true, ".nuxt": true, ".turbo": true, ".cache": true,
	"coverage": true, "__pycache__": true, ".venv": true, "venv": true,
	".tox": true, ".mypy_cache": true, ".pytest_cache": true,
	".idea": true, ".vscode": true, ".jobs": true,
	"Pods": true, "Carthage": true,
}

// ignoreFileNames skipped as noise when sampling.
var ignoreFileNames = map[string]bool{
	".DS_Store": true, "Thumbs.db": true,
}

func shouldSkipDir(name string) bool {
	if ignoreDirNames[name] {
		return true
	}
	// skip hidden dirs except .agents / .github (useful signals)
	if strings.HasPrefix(name, ".") && name != ".agents" && name != ".github" {
		return true
	}
	return false
}

func shouldSkipFile(name string) bool {
	return ignoreFileNames[name]
}

// importantDirNames get expanded samples in the map.
var importantDirNames = map[string]bool{
	"cmd": true, "internal": true, "pkg": true, "src": true, "app": true,
	"apps": true, "packages": true, "lib": true, "server": true, "client": true,
	"api": true, "web": true, "docs": true, "scripts": true, "deploy": true,
	"services": true, "backend": true, "frontend": true, "crates": true,
	".agents": true, ".github": true,
}

func isImportantDir(rel string) bool {
	base := filepath.Base(rel)
	if importantDirNames[base] {
		return true
	}
	// top-level always somewhat important
	if !strings.Contains(rel, string(filepath.Separator)) && rel != "." {
		return true
	}
	return false
}
