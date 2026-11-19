package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunBuildsThePackage: the command builds the package from the
// repository's sources into the output path, creating its directory, and
// a repository without the notice file is refused naming it.
func TestRunBuildsThePackage(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bin", "tcgreplay-plugin.zip")
	if err := run("../..", out); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if len(zr.File) != 3 {
		t.Errorf("package has %d entries, want 3", len(zr.File))
	}

	// A copy of the sources without internal/notice/notice.json.
	root := t.TempDir()
	for _, rel := range []string{".do/app.yaml", "plugin/listing.json", "plugin/release-notes.md", "plugin/assets/icon.png"} {
		b, err := os.ReadFile(filepath.Join("../..", rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := run(root, filepath.Join(root, "out.zip")); err == nil || !strings.Contains(err.Error(), "notice.json") {
		t.Errorf("build without the notice file: %v, want an error naming notice.json", err)
	}
}
