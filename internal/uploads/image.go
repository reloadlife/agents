// Package uploads saves clipboard/drag images into a workspace for agent CLIs.
package uploads

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

const maxImageBytes = 12 << 20 // 12 MiB

// Result of a successful image save.
type Result struct {
	// Abs is the absolute path on the agents host.
	Abs string `json:"abs"`
	// Rel is workspace-relative path (forward slashes).
	Rel string `json:"rel"`
	// CwdRel is path relative to the session cwd (best for pasting into the agent).
	CwdRel string `json:"cwd_rel"`
	// Bytes written.
	Bytes int `json:"bytes"`
	// MIME type normalized.
	MIME string `json:"mime"`
}

// SaveImage writes base64 image data under absCwd/.agents/pastes/.
// dataURL may be raw base64 or a data:image/...;base64,... URL.
func SaveImage(workspaceRoot, absCwd, relCwd, mime, data string) (*Result, error) {
	raw, mimeOut, err := decodeImage(mime, data)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty image")
	}
	if len(raw) > maxImageBytes {
		return nil, fmt.Errorf("image too large (%d bytes, max %d)", len(raw), maxImageBytes)
	}
	ext := extForMIME(mimeOut)
	dir := filepath.Join(absCwd, ".agents", "pastes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("paste-%s-%s%s",
		time.Now().UTC().Format("20060102-150405"),
		ulid.Make().String()[:10],
		ext,
	)
	abs := filepath.Join(dir, name)
	if err := os.WriteFile(abs, raw, 0o644); err != nil {
		return nil, err
	}
	relFromWS := filepath.ToSlash(filepath.Join(relCwd, ".agents", "pastes", name))
	if relCwd == "." || relCwd == "" {
		relFromWS = filepath.ToSlash(filepath.Join(".agents", "pastes", name))
	}
	cwdRel := filepath.ToSlash(filepath.Join(".agents", "pastes", name))
	return &Result{
		Abs:    abs,
		Rel:    relFromWS,
		CwdRel: cwdRel,
		Bytes:  len(raw),
		MIME:   mimeOut,
	}, nil
}

func decodeImage(mime, data string) ([]byte, string, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, "", fmt.Errorf("no image data")
	}
	// data URL
	if strings.HasPrefix(data, "data:") {
		// data:image/png;base64,AAAA
		comma := strings.IndexByte(data, ',')
		if comma < 0 {
			return nil, "", fmt.Errorf("invalid data URL")
		}
		header := data[5:comma]
		payload := data[comma+1:]
		if strings.Contains(header, ";base64") {
			mimePart := strings.Split(header, ";")[0]
			if mimePart != "" {
				mime = mimePart
			}
			b, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				// try raw std without padding issues
				b, err = base64.RawStdEncoding.DecodeString(payload)
			}
			if err != nil {
				return nil, "", fmt.Errorf("base64 decode: %w", err)
			}
			return b, normalizeMIME(mime), nil
		}
		return nil, "", fmt.Errorf("data URL must be base64")
	}
	b, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		b, err = base64.RawStdEncoding.DecodeString(data)
	}
	if err != nil {
		return nil, "", fmt.Errorf("base64 decode: %w", err)
	}
	return b, normalizeMIME(mime), nil
}

func normalizeMIME(m string) string {
	m = strings.ToLower(strings.TrimSpace(m))
	if m == "" {
		return "image/png"
	}
	switch m {
	case "image/jpg":
		return "image/jpeg"
	case "image/x-png":
		return "image/png"
	}
	if !strings.HasPrefix(m, "image/") {
		return "image/png"
	}
	return m
}

func extForMIME(m string) string {
	switch m {
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/svg+xml":
		return ".svg"
	case "image/bmp":
		return ".bmp"
	default:
		return ".png"
	}
}
