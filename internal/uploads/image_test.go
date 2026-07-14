package uploads

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveImageDataURL(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "proj")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	data := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	res, err := SaveImage(root, cwd, "proj", "image/png", data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Abs); err != nil {
		t.Fatal(err)
	}
	if res.CwdRel == "" || res.Bytes < 10 {
		t.Fatalf("%+v", res)
	}
}
