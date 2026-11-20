// Command tcgreplay-plugin builds the ChatGPT plugin package for OpenAI's
// plugin dashboard (see package plugin) from the repository's sources:
// .do/app.yaml, internal/notice/notice.json, plugin/listing.json,
// plugin/release-notes.md and plugin/assets/icon.png. Run it with
// `make plugin`.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nicholasjackson/tcgreplay/internal/plugin"
)

func main() {
	root := flag.String("root", ".", "the repository root to read the sources from")
	out := flag.String("out", "bin/tcgreplay-plugin.zip", "where to write the plugin package")
	flag.Parse()
	if err := run(*root, *out); err != nil {
		fmt.Fprintln(os.Stderr, "tcgreplay-plugin:", err)
		os.Exit(1)
	}
}

func run(root, out string) error {
	read := func(rel string) ([]byte, error) { return os.ReadFile(filepath.Join(root, rel)) }
	var src plugin.Sources
	for _, f := range []struct {
		rel string
		set func([]byte)
	}{
		{".do/app.yaml", func(b []byte) { src.AppSpec = string(b) }},
		{"plugin/listing.json", func(b []byte) { src.Listing = string(b) }},
		{"plugin/release-notes.md", func(b []byte) { src.ReleaseNotes = string(b) }},
		{"plugin/assets/icon.png", func(b []byte) { src.Icon = b }},
	} {
		b, err := read(f.rel)
		if err != nil {
			return err
		}
		f.set(b)
	}
	// A missing notice file is left to Build, which names the rule.
	if b, err := read("internal/notice/notice.json"); err == nil {
		src.Notice = string(b)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	zip, err := plugin.Build(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(out, zip, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", out, len(zip))
	return nil
}
