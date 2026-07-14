package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v0.2.2":           "0.2.2",
		"0.2.2":            "0.2.2",
		"v0.2.2-dirty":     "0.2.2",
		"v0.2.2-3-gabcdef": "0.2.2",
		"  v1.0.0  ":       "1.0.0",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q)=%q want %q", in, got, want)
		}
	}
}

func TestVersionsEqual(t *testing.T) {
	if !versionsEqual("v0.2.2", "0.2.2") {
		t.Fatal("expected equal")
	}
	if !versionsEqual("v0.2.2-dirty", "v0.2.2") {
		t.Fatal("dirty should equal base")
	}
	if versionsEqual("v0.2.1", "v0.2.2") {
		t.Fatal("different versions")
	}
}

func TestLooksDev(t *testing.T) {
	if !looksDev("dev") || !looksDev("v0.2.2-dirty") {
		t.Fatal("expected dev")
	}
	if looksDev("v0.2.2") {
		t.Fatal("release is not dev")
	}
}

func TestExtractAndFind(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := writeTestArchive(archive, map[string]string{
		"agents_v0.2.2_linux_amd64/agentsd":   "#!/bin/sh\necho d\n",
		"agents_v0.2.2_linux_amd64/agentsctl": "#!/bin/sh\necho c\n",
	}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatal(err)
	}
	found, err := findExtractedBins(dest, []string{"agentsd", "agentsctl"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("found=%v", found)
	}
}

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceFile(dst, src); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "new" {
		t.Fatalf("got %q", b)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o111 == 0 {
		t.Fatalf("not executable: %v", st.Mode())
	}
}

func writeTestArchive(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	for name, body := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(body)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.WriteString(tw, body); err != nil {
			return err
		}
	}
	return nil
}

func TestWhichBinaries(t *testing.T) {
	if got := whichBinaries(Options{Binary: "agentsctl"}); len(got) != 1 || got[0] != "agentsctl" {
		t.Fatalf("%v", got)
	}
	if got := whichBinaries(Options{All: true}); len(got) != 2 {
		t.Fatalf("%v", got)
	}
}
