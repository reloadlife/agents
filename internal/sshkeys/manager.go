// Package sshkeys manages OpenSSH identity files under the agentsd user's ~/.ssh.
// Private keys are never returned over the API — only metadata and public keys.
package sshkeys

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Protected basenames cannot be deleted via the API.
var protectedNames = map[string]bool{
	"authorized_keys":     true,
	"authorized_keys.bak": true,
	"known_hosts":         true,
	"known_hosts.old":     true,
	"config":              true,
}

var safeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Key is a public identity (private material never included).
type Key struct {
	Name        string    `json:"name"`                   // file basename (no .pub)
	Path        string    `json:"path"`                   // absolute private path (existence only)
	PublicPath  string    `json:"public_path"`            // absolute .pub path
	HasPrivate  bool      `json:"has_private"`            // private file present
	HasPublic   bool      `json:"has_public"`             // .pub present
	PublicKey   string    `json:"public_key,omitempty"`   // full .pub line
	Fingerprint string    `json:"fingerprint,omitempty"`  // ssh-keygen -lf
	Comment     string    `json:"comment,omitempty"`      // trailing comment on pub line
	Type        string    `json:"type,omitempty"`         // e.g. ED25519, RSA
	Bits        string    `json:"bits,omitempty"`         // from fingerprint line
	ModTime     time.Time `json:"mod_time,omitempty"`
	Protected   bool      `json:"protected,omitempty"` // cannot delete via API
}

// GenerateRequest creates a new key pair.
type GenerateRequest struct {
	// Name: identity file name under ~/.ssh (e.g. id_agents_github). Required.
	Name string `json:"name"`
	// Type: ed25519 (default) or rsa
	Type string `json:"type,omitempty"`
	// Comment: typically email or host label
	Comment string `json:"comment,omitempty"`
	// Bits: RSA only (default 4096)
	Bits int `json:"bits,omitempty"`
	// Overwrite: replace existing pair
	Overwrite bool `json:"overwrite,omitempty"`
}

// Manager operates on a single SSH directory (usually $HOME/.ssh).
type Manager struct {
	dir string
}

// New returns a manager for dir (created if missing, mode 0700).
func New(dir string) (*Manager, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("home dir: %w", err)
		}
		dir = filepath.Join(home, ".ssh")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	return &Manager{dir: abs}, nil
}

func (m *Manager) Dir() string { return m.dir }

// List returns identity keys found under the SSH dir.
func (m *Manager) List() ([]Key, error) {
	ents, err := os.ReadDir(m.dir)
	if err != nil {
		return nil, err
	}
	// group by base name without .pub
	seen := map[string]bool{}
	var names []string
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if n == "" || strings.HasPrefix(n, ".") {
			continue
		}
		// skip non-key clutter
		if n == "config" || strings.HasPrefix(n, "known_hosts") || strings.HasPrefix(n, "authorized_keys") {
			// still show as protected entries if .pub-less system files — skip listing them as keys
			continue
		}
		base := strings.TrimSuffix(n, ".pub")
		if !seen[base] {
			seen[base] = true
			names = append(names, base)
		}
	}
	sort.Strings(names)
	out := make([]Key, 0, len(names))
	for _, name := range names {
		k, err := m.Get(name)
		if err != nil {
			continue
		}
		// skip entries with neither pub nor private (shouldn't happen)
		if !k.HasPrivate && !k.HasPublic {
			continue
		}
		out = append(out, *k)
	}
	return out, nil
}

// Get returns metadata + public key for name.
func (m *Manager) Get(name string) (*Key, error) {
	name, err := sanitizeName(name)
	if err != nil {
		return nil, err
	}
	priv := filepath.Join(m.dir, name)
	pub := priv + ".pub"
	k := &Key{
		Name:       name,
		Path:       priv,
		PublicPath: pub,
		Protected:  protectedNames[name],
	}
	if st, err := os.Stat(priv); err == nil && !st.IsDir() {
		k.HasPrivate = true
		k.ModTime = st.ModTime().UTC()
	}
	if st, err := os.Stat(pub); err == nil && !st.IsDir() {
		k.HasPublic = true
		if k.ModTime.IsZero() || st.ModTime().After(k.ModTime) {
			k.ModTime = st.ModTime().UTC()
		}
		b, err := os.ReadFile(pub)
		if err == nil {
			line := strings.TrimSpace(string(b))
			k.PublicKey = line
			k.Comment = pubComment(line)
			k.Type = pubType(line)
		}
		if fp, bits, typ := fingerprint(pub); fp != "" {
			k.Fingerprint = fp
			if bits != "" {
				k.Bits = bits
			}
			if typ != "" && k.Type == "" {
				k.Type = typ
			}
		}
	}
	return k, nil
}

// Generate creates a new key pair with ssh-keygen. Returns public metadata only.
func (m *Manager) Generate(req GenerateRequest) (*Key, error) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		return nil, fmt.Errorf("ssh-keygen not found on PATH")
	}
	name, err := sanitizeName(req.Name)
	if err != nil {
		return nil, err
	}
	if protectedNames[name] {
		return nil, fmt.Errorf("name %q is reserved", name)
	}
	priv := filepath.Join(m.dir, name)
	pub := priv + ".pub"
	if !req.Overwrite {
		if _, err := os.Stat(priv); err == nil {
			return nil, fmt.Errorf("key %q already exists (set overwrite=true to replace)", name)
		}
		if _, err := os.Stat(pub); err == nil {
			return nil, fmt.Errorf("public key %q already exists", name+".pub")
		}
	} else {
		_ = os.Remove(priv)
		_ = os.Remove(pub)
	}

	typ := strings.ToLower(strings.TrimSpace(req.Type))
	if typ == "" {
		typ = "ed25519"
	}
	comment := strings.TrimSpace(req.Comment)
	if comment == "" {
		host, _ := os.Hostname()
		comment = "agents@" + host
	}

	args := []string{"-q", "-t", typ, "-C", comment, "-f", priv, "-N", ""}
	if typ == "rsa" {
		bits := req.Bits
		if bits <= 0 {
			bits = 4096
		}
		args = append(args, "-b", fmt.Sprintf("%d", bits))
	} else if typ != "ed25519" && typ != "ecdsa" {
		return nil, fmt.Errorf("unsupported key type %q (use ed25519 or rsa)", typ)
	}

	cmd := exec.Command("ssh-keygen", args...)
	cmd.Dir = m.dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ssh-keygen: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// enforce modes
	_ = os.Chmod(priv, 0o600)
	_ = os.Chmod(pub, 0o644)
	return m.Get(name)
}

// Delete removes private + public files for name.
func (m *Manager) Delete(name string) error {
	name, err := sanitizeName(name)
	if err != nil {
		return err
	}
	if protectedNames[name] {
		return fmt.Errorf("refusing to delete protected file %q", name)
	}
	priv := filepath.Join(m.dir, name)
	pub := priv + ".pub"
	var removed bool
	if err := os.Remove(priv); err == nil {
		removed = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Remove(pub); err == nil {
		removed = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if !removed {
		return fmt.Errorf("key not found: %s", name)
	}
	return nil
}

func sanitizeName(name string) (string, error) {
	raw := strings.TrimSpace(name)
	if raw == "" {
		return "", fmt.Errorf("invalid key name")
	}
	// reject any path-shaped input before Base()
	if strings.Contains(raw, "/") || strings.Contains(raw, `\`) || strings.Contains(raw, "..") {
		return "", fmt.Errorf("invalid key name")
	}
	name = filepath.Base(raw)
	name = strings.TrimSuffix(name, ".pub")
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("invalid key name")
	}
	if !safeName.MatchString(name) {
		return "", fmt.Errorf("invalid key name %q (use letters, numbers, . _ -)", name)
	}
	return name, nil
}

func pubComment(line string) string {
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return strings.Join(parts[2:], " ")
	}
	return ""
}

func pubType(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}
	// ssh-ed25519 → ED25519
	t := parts[0]
	t = strings.TrimPrefix(t, "ssh-")
	t = strings.TrimPrefix(t, "ecdsa-sha2-")
	return strings.ToUpper(t)
}

func fingerprint(pubPath string) (fp, bits, typ string) {
	out, err := exec.Command("ssh-keygen", "-lf", pubPath).CombinedOutput()
	if err != nil {
		return "", "", ""
	}
	// e.g. "256 SHA256:abc… comment (ED25519)"
	line := strings.TrimSpace(string(out))
	fields := strings.Fields(line)
	if len(fields) >= 2 {
		bits = fields[0]
		fp = fields[1]
	}
	if i := strings.LastIndex(line, "("); i >= 0 && strings.HasSuffix(line, ")") {
		typ = strings.TrimSuffix(line[i+1:], ")")
	}
	return fp, bits, typ
}
