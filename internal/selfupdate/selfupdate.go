// Package selfupdate downloads GitHub release binaries and replaces the
// running agentsd / agentsctl executable in place.
package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const DefaultRepo = "reloadlife/agents"

// Options control a self-update run.
type Options struct {
	// Repo is "owner/name". Empty → DefaultRepo.
	Repo string
	// Current is the binary's embedded version string (e.g. v0.2.2-dirty).
	Current string
	// Binary is the name to install: "agentsctl", "agentsd", or "all".
	// Empty → "all" when All is true, else must be set by caller.
	Binary string
	// All updates both agentsd and agentsctl when present in the archive
	// (and, for the sibling, when already installed next to the executable).
	All bool
	// Version pins a release tag (e.g. v0.2.2). Empty → latest.
	Version string
	// CheckOnly reports whether an update is available without installing.
	CheckOnly bool
	// Force reinstalls even when the resolved tag matches Current.
	Force bool
	// Client is optional; default has a 2m timeout.
	Client *http.Client
	// Stdout / Stderr default to os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer
}

// Result is returned after a successful Run (including check-only).
type Result struct {
	Current   string
	Latest    string
	Updated   bool
	Binaries  []string // paths written
	AssetURL  string
	CheckOnly bool
}

// Run resolves a release, optionally downloads it, and replaces binaries.
func Run(opts Options) (*Result, error) {
	out := opts.Stdout
	if out == nil {
		out = os.Stdout
	}
	errOut := opts.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	repo := opts.Repo
	if repo == "" {
		repo = DefaultRepo
	}
	hc := opts.Client
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Minute}
	}

	want := whichBinaries(opts)
	if len(want) == 0 {
		return nil, fmt.Errorf("nothing to update (set Binary to agentsctl or agentsd, or All)")
	}

	tag, err := resolveTag(hc, repo, opts.Version)
	if err != nil {
		return nil, err
	}
	cur := normalizeVersion(opts.Current)
	lat := normalizeVersion(tag)

	res := &Result{
		Current:   opts.Current,
		Latest:    tag,
		CheckOnly: opts.CheckOnly,
	}

	same := versionsEqual(cur, lat)
	if opts.CheckOnly {
		if same && !looksDev(opts.Current) {
			fmt.Fprintf(out, "up to date: %s (latest %s)\n", opts.Current, tag)
		} else {
			fmt.Fprintf(out, "update available: %s → %s\n", opts.Current, tag)
			res.Updated = true // means "update available" in check mode
		}
		return res, nil
	}

	if same && !opts.Force && !looksDev(opts.Current) {
		fmt.Fprintf(out, "already on %s (latest %s) — use --force to reinstall\n", opts.Current, tag)
		return res, nil
	}

	osName, arch, err := platform()
	if err != nil {
		return nil, err
	}
	assetURL, err := findAssetURL(hc, repo, tag, osName, arch)
	if err != nil {
		return nil, err
	}
	res.AssetURL = assetURL

	fmt.Fprintf(errOut, "→ downloading %s\n", assetURL)
	tmpDir, err := os.MkdirTemp("", "agents-update-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "release.tar.gz")
	if err := downloadFile(hc, assetURL, archivePath); err != nil {
		return nil, err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return nil, err
	}
	if err := extractTarGz(archivePath, extractDir); err != nil {
		return nil, err
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return nil, fmt.Errorf("resolve executable: %w", err)
	}
	binDir := filepath.Dir(exe)

	// Map of binary name → path inside extract tree
	found, err := findExtractedBins(extractDir, want)
	if err != nil {
		return nil, err
	}

	var written []string
	for _, name := range want {
		src, ok := found[name]
		if !ok {
			return nil, fmt.Errorf("release archive missing %s", name)
		}
		dst := filepath.Join(binDir, name)
		if filepath.Base(exe) == name {
			dst = exe // replace the running binary path (handles nonstandard names)
		} else if opts.All {
			// only install sibling if already present next to us
			if _, err := os.Stat(dst); err != nil {
				fmt.Fprintf(errOut, "→ skip %s (not installed at %s)\n", name, dst)
				continue
			}
		}
		fmt.Fprintf(errOut, "→ installing %s → %s\n", name, dst)
		if err := replaceFile(dst, src); err != nil {
			return nil, fmt.Errorf("install %s: %w", name, err)
		}
		written = append(written, dst)
	}

	if len(written) == 0 {
		return nil, fmt.Errorf("no binaries installed")
	}

	res.Updated = true
	res.Binaries = written
	fmt.Fprintf(out, "updated to %s\n", tag)
	for _, p := range written {
		fmt.Fprintf(out, "  %s\n", p)
	}
	if containsBase(written, "agentsd") {
		fmt.Fprintln(out, "restart agentsd to run the new binary:")
		fmt.Fprintln(out, "  systemctl --user restart agentsd   # user unit")
		fmt.Fprintln(out, "  # or:  sudo systemctl restart agentsd")
	}
	return res, nil
}

func whichBinaries(opts Options) []string {
	if opts.All {
		return []string{"agentsd", "agentsctl"}
	}
	switch strings.TrimSpace(opts.Binary) {
	case "agentsd", "agentsctl":
		return []string{opts.Binary}
	case "all":
		return []string{"agentsd", "agentsctl"}
	default:
		return nil
	}
}

func containsBase(paths []string, base string) bool {
	for _, p := range paths {
		if filepath.Base(p) == base {
			return true
		}
	}
	return false
}

func platform() (osName, arch string, err error) {
	osName = runtime.GOOS
	switch osName {
	case "linux", "darwin":
	default:
		return "", "", fmt.Errorf("unsupported OS %s (want linux|darwin)", osName)
	}
	switch runtime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported arch %s (want amd64|arm64)", runtime.GOARCH)
	}
	return osName, arch, nil
}

// resolveTag returns a release tag. want empty → latest.
func resolveTag(hc *http.Client, repo, want string) (string, error) {
	want = strings.TrimSpace(want)
	if want != "" && want != "latest" {
		if !strings.HasPrefix(want, "v") && isNumericVersion(want) {
			want = "v" + want
		}
		return want, nil
	}
	// Prefer GitHub API (JSON); fall back to releases/latest redirect.
	apiURL := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "agents-selfupdate")
	res, err := hc.Do(req)
	if err == nil {
		defer res.Body.Close()
		if res.StatusCode == 200 {
			var body struct {
				TagName string `json:"tag_name"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err == nil && body.TagName != "" {
				return body.TagName, nil
			}
		}
	}

	// Redirect probe (no API rate limit token needed for this either, usually).
	client := &http.Client{
		Timeout: hc.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	r2, err := client.Head("https://github.com/" + repo + "/releases/latest")
	if err != nil {
		return "", fmt.Errorf("resolve latest release: %w", err)
	}
	defer r2.Body.Close()
	loc := r2.Header.Get("Location")
	if loc == "" {
		// Some environments follow anyway — try GET body path.
		r3, err := client.Get("https://github.com/" + repo + "/releases/latest")
		if err != nil {
			return "", fmt.Errorf("resolve latest release: %w", err)
		}
		defer r3.Body.Close()
		loc = r3.Header.Get("Location")
		if loc == "" && r3.Request != nil && r3.Request.URL != nil {
			loc = r3.Request.URL.String()
		}
	}
	if loc == "" {
		return "", fmt.Errorf("could not resolve latest release for %s", repo)
	}
	// .../releases/tag/v0.2.2
	parts := strings.Split(strings.TrimRight(loc, "/"), "/")
	tag := parts[len(parts)-1]
	if tag == "" || tag == "latest" {
		return "", fmt.Errorf("could not parse release tag from %s", loc)
	}
	return tag, nil
}

func findAssetURL(hc *http.Client, repo, tag, osName, arch string) (string, error) {
	names := []string{
		fmt.Sprintf("agents_%s_%s_%s.tar.gz", tag, osName, arch),
		fmt.Sprintf("local-agents_%s_%s_%s.tar.gz", tag, osName, arch),
	}
	// Try API asset list first for accuracy.
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", repo, tag)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "agents-selfupdate")
		if res, err := hc.Do(req); err == nil {
			defer res.Body.Close()
			if res.StatusCode == 200 {
				var body struct {
					Assets []struct {
						Name               string `json:"name"`
						BrowserDownloadURL string `json:"browser_download_url"`
					} `json:"assets"`
				}
				if err := json.NewDecoder(res.Body).Decode(&body); err == nil {
					byName := map[string]string{}
					for _, a := range body.Assets {
						byName[a.Name] = a.BrowserDownloadURL
					}
					for _, n := range names {
						if u, ok := byName[n]; ok {
							return u, nil
						}
					}
				}
			}
		}
	}
	// Direct URL fallback.
	for _, n := range names {
		u := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, n)
		if headOK(hc, u) {
			return u, nil
		}
	}
	return "", fmt.Errorf("no release asset for %s/%s %s (tried %s)", osName, arch, tag, strings.Join(names, ", "))
}

func headOK(hc *http.Client, url string) bool {
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "agents-selfupdate")
	res, err := hc.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode >= 200 && res.StatusCode < 300
}

func downloadFile(hc *http.Client, url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "agents-selfupdate")
	res, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("download HTTP %d: %s", res.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func extractTarGz(archive, destDir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Prevent zip-slip
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("refusing unsafe path in archive: %s", hdr.Name)
		}
		target := filepath.Join(destDir, name)
		if !strings.HasPrefix(target, filepath.Clean(destDir)+string(os.PathSeparator)) && target != filepath.Clean(destDir) {
			return fmt.Errorf("refusing path escape: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func findExtractedBins(root string, want []string) (map[string]string, error) {
	need := map[string]bool{}
	for _, w := range want {
		need[w] = true
	}
	found := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if need[base] {
			found[base] = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, w := range want {
		if _, ok := found[w]; !ok {
			return nil, fmt.Errorf("archive missing binary %q", w)
		}
	}
	return found, nil
}

// replaceFile writes src over dst atomically (same directory temp + rename).
func replaceFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Ensure cleanup on failure
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	// On Unix, rename over a running binary is fine.
	if err := os.Rename(tmpName, dst); err != nil {
		// Cross-device: copy + remove
		if err2 := copyFile(tmpName, dst); err2 != nil {
			return err
		}
		_ = os.Remove(tmpName)
	}
	success = true
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// normalizeVersion strips common suffixes for comparison.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "+"); i >= 0 {
		v = v[:i]
	}
	// v0.2.2-dirty → 0.2.2 ; v0.2.2-3-gabc → 0.2.2
	// keep pre-releases like 0.3.0-rc.1
	if i := strings.Index(v, "-"); i >= 0 {
		if looksGitDescribe(v[i+1:]) {
			v = v[:i]
		}
	}
	return strings.TrimSpace(v)
}

func looksGitDescribe(rest string) bool {
	if rest == "dirty" || strings.HasPrefix(rest, "dirty") {
		return true
	}
	// N-gHEX (git describe)
	parts := strings.Split(rest, "-")
	if len(parts) >= 2 {
		last := parts[len(parts)-1]
		if strings.HasPrefix(last, "g") && len(last) > 1 {
			return true
		}
	}
	if strings.HasPrefix(rest, "g") && len(rest) > 1 {
		return true
	}
	return false
}

func isNumericVersion(s string) bool {
	s = strings.TrimPrefix(s, "v")
	parts := strings.Split(s, ".")
	if len(parts) < 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

func versionsEqual(a, b string) bool {
	return normalizeVersion(a) == normalizeVersion(b) && normalizeVersion(a) != ""
}

func looksDev(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "" || v == "dev" || v == "none" || v == "unknown" || strings.Contains(v, "dirty")
}
