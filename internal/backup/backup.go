// Package backup creates and restores host state tarballs.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Create writes a gzip tar of sessions metadata, memory dir, templates, audit, recordings.
func Create(jobsDir, outPath string) (string, error) {
	if outPath == "" {
		outPath = filepath.Join(jobsDir, "backups", fmt.Sprintf("agents-backup-%s.tar.gz", time.Now().UTC().Format("20060102-150405")))
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o700); err != nil {
		return "", err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	roots := []string{
		filepath.Join(jobsDir, "sessions"),
		filepath.Join(jobsDir, "memory"),
		filepath.Join(jobsDir, "templates"),
		filepath.Join(jobsDir, "audit"),
		filepath.Join(jobsDir, "recordings"),
	}
	for _, root := range roots {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			rel, err := filepath.Rel(jobsDir, path)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return nil
			}
			hdr.Name = rel
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			rf, err := os.Open(path)
			if err != nil {
				return nil
			}
			_, _ = io.Copy(tw, rf)
			_ = rf.Close()
			return nil
		})
	}
	return outPath, nil
}

// Restore extracts a backup tarball into jobsDir (overwrites matching files).
func Restore(jobsDir, tarPath string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
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
		name := filepath.Clean(hdr.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			continue
		}
		target := filepath.Join(jobsDir, name)
		if hdr.FileInfo().IsDir() {
			_ = os.MkdirAll(target, 0o700)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		wf, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, err = io.Copy(wf, tr)
		_ = wf.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
