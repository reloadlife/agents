package workspaces

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reloadlife/agents/internal/config"
)

func testCfg(root string) *config.Config {
	return &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"."}},
		Sessions:      config.SessionsConfig{SSHHost: "user@agents"},
	}
}

func TestOpenFolderCommands(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "agents")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := Open(testCfg(root), OpenRequest{Cwd: "agents"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Cwd != "agents" {
		t.Fatalf("cwd=%q", out.Cwd)
	}
	if out.Abs != ws {
		t.Fatalf("abs=%q want %q", out.Abs, ws)
	}
	if out.Path != "" || out.Line != 0 {
		t.Fatalf("unexpected path/line: %q %d", out.Path, out.Line)
	}
	if out.SSHHost != "user@agents" {
		t.Fatalf("ssh_host=%q", out.SSHHost)
	}
	// Host label for vscode-remote strips user@
	if !strings.Contains(out.Commands["cursor_remote"], "ssh-remote+agents") {
		t.Fatalf("cursor_remote=%q", out.Commands["cursor_remote"])
	}
	if !strings.Contains(out.Commands["cursor_remote"], "--folder-uri") {
		t.Fatalf("expected folder-uri: %q", out.Commands["cursor_remote"])
	}
	if !strings.Contains(out.Commands["vscode_remote"], "--folder-uri") {
		t.Fatalf("expected folder-uri: %q", out.Commands["vscode_remote"])
	}
	if !strings.Contains(out.Commands["zed_remote"], "ssh://user@agents") {
		t.Fatalf("zed_remote=%q", out.Commands["zed_remote"])
	}
	if !strings.Contains(out.Commands["cursor_local"], ws) {
		t.Fatalf("cursor_local=%q", out.Commands["cursor_local"])
	}
}

func TestOpenFileWithLine(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "agents")
	fileRel := filepath.Join("web", "src", "main.ts")
	fileAbs := filepath.Join(ws, fileRel)
	if err := os.MkdirAll(filepath.Dir(fileAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileAbs, []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Open(testCfg(root), OpenRequest{
		Cwd:  "agents",
		Path: "web/src/main.ts",
		Line: 120,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Path != "web/src/main.ts" {
		t.Fatalf("path=%q", out.Path)
	}
	if out.Line != 120 {
		t.Fatalf("line=%d", out.Line)
	}
	// Abs stays workspace dir
	if out.Abs != ws {
		t.Fatalf("abs=%q want workspace %q", out.Abs, ws)
	}

	fileSlash := filepath.ToSlash(fileAbs)
	// Remote: --goto with path:line
	cr := out.Commands["cursor_remote"]
	if !strings.Contains(cr, "--goto") {
		t.Fatalf("cursor_remote missing --goto: %q", cr)
	}
	if !strings.Contains(cr, fileSlash+":120") {
		t.Fatalf("cursor_remote missing file:line: %q", cr)
	}
	vr := out.Commands["vscode_remote"]
	if !strings.Contains(vr, "--goto") || !strings.Contains(vr, fileSlash+":120") {
		t.Fatalf("vscode_remote=%q", vr)
	}
	// file-uri variant without line
	if !strings.Contains(out.Commands["cursor_remote_file"], "--file-uri") {
		t.Fatalf("cursor_remote_file=%q", out.Commands["cursor_remote_file"])
	}
	if !strings.Contains(out.Commands["cursor_remote_file"], fileSlash) {
		t.Fatalf("cursor_remote_file missing file: %q", out.Commands["cursor_remote_file"])
	}
	// folder still present
	if !strings.Contains(out.Commands["cursor_remote_folder"], "--folder-uri") {
		t.Fatalf("cursor_remote_folder=%q", out.Commands["cursor_remote_folder"])
	}
	// Local -g with :line
	if !strings.Contains(out.Commands["cursor_local"], "-g") {
		t.Fatalf("cursor_local=%q", out.Commands["cursor_local"])
	}
	if !strings.Contains(out.Commands["cursor_local"], fileAbs+":120") {
		t.Fatalf("cursor_local missing file:line: %q", out.Commands["cursor_local"])
	}
	if !strings.Contains(out.Commands["vscode_local"], "-g") {
		t.Fatalf("vscode_local=%q", out.Commands["vscode_local"])
	}
	if !strings.Contains(out.Commands["zed_local"], fileAbs+":120") {
		t.Fatalf("zed_local=%q", out.Commands["zed_local"])
	}
	if !strings.Contains(out.Commands["zed_remote"], fileSlash+":120") {
		t.Fatalf("zed_remote=%q", out.Commands["zed_remote"])
	}
}

func TestOpenFileNoLine(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "repo")
	fileAbs := filepath.Join(ws, "a.go")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fileAbs, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := Open(testCfg(root), OpenRequest{Cwd: "repo", Path: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Line != 0 {
		t.Fatalf("line should be omitted/zero, got %d", out.Line)
	}
	// Local without -g when no line
	if strings.Contains(out.Commands["cursor_local"], "-g") {
		t.Fatalf("unexpected -g: %q", out.Commands["cursor_local"])
	}
	if !strings.Contains(out.Commands["cursor_local"], fileAbs) {
		t.Fatalf("cursor_local=%q", out.Commands["cursor_local"])
	}
	// --goto still used for remote file (resource is path without :line)
	if !strings.Contains(out.Commands["cursor_remote"], "--goto") {
		t.Fatalf("cursor_remote=%q", out.Commands["cursor_remote"])
	}
	if strings.Contains(out.Commands["cursor_remote"], ":0") {
		t.Fatalf("unexpected :0 in remote: %q", out.Commands["cursor_remote"])
	}
}

func TestOpenPathEscapeRejected(t *testing.T) {
	root := t.TempDir()
	ws := filepath.Join(root, "agents")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		"../etc/passwd",
		"/etc/passwd",
		"web/../../etc/passwd",
		"..",
	}
	for _, p := range cases {
		_, err := Open(testCfg(root), OpenRequest{Cwd: "agents", Path: p})
		if err == nil {
			t.Fatalf("path %q: expected error", p)
		}
	}
}

func TestResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	abs, err := resolveUnderRoot(root, "sub/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "sub", "file.txt")
	if abs != want {
		t.Fatalf("got %q want %q", abs, want)
	}

	if _, err := resolveUnderRoot(root, "../x"); err == nil {
		t.Fatal("expected .. reject")
	}
	if _, err := resolveUnderRoot(root, "/abs"); err == nil {
		t.Fatal("expected abs reject")
	}
}

func TestLocalLaunchArgs(t *testing.T) {
	// Only checks arg construction when binary missing → empty.
	// With unknown editor:
	bin, args := localLaunch("nope", "/tmp/x", 10)
	if bin != "" || args != nil {
		t.Fatalf("expected empty for unknown editor")
	}
}

func TestOpenAllowlist(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ok"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nope"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		WorkspaceRoot: root,
		Allow:         config.AllowConfig{Paths: []string{"ok"}},
		Sessions:      config.SessionsConfig{SSHHost: "agents"},
	}
	if _, err := Open(cfg, OpenRequest{Cwd: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(cfg, OpenRequest{Cwd: "nope"}); err == nil {
		t.Fatal("expected allowlist reject")
	}
}
