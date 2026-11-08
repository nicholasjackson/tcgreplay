package catalog

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tcgdexAPI is the upstream card API's host. Only the maintainer's card-set
// import may name it: the playmat reads card data from published sets.
const tcgdexAPI = "api.tcgdex.net"

// apiAllowed lists, by file, the only program code allowed to name the
// TCGdex API: the card-set import's fetcher behind `tcgreplay sets
// import`. Never a directory or package wildcard.
var apiAllowed = []string{"internal/cardset/tcgdex.go"}

// namesAPI walks the non-test Go files under each dir of root (paths
// relative to root) and returns those, other than the allowed files, that
// name the TCGdex API.
func namesAPI(t *testing.T, root string, dirs, allowed []string) []string {
	t.Helper()
	var hits []string
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "node_modules" || name == "testdata" || (strings.HasPrefix(name, ".") && p != filepath.Join(root, dir)) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if slices.Contains(allowed, rel) {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(string(b), tcgdexAPI) {
				hits = append(hits, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return hits
}

// TestOnlyTheImportFetcherNamesTheTCGdexAPI: no program code but the
// card-set import's TCGdex fetcher names the upstream card API, so nothing
// the playmat runs (internal/catalog, internal/cards, internal/game, the
// board and MCP servers, cmd/) can ask it for card data.
func TestOnlyTheImportFetcherNamesTheTCGdexAPI(t *testing.T) {
	root := filepath.Join("..", "..")
	for _, f := range apiAllowed {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Fatalf("allowed file %s is gone: update apiAllowed (%v)", f, err)
		}
	}
	for _, f := range namesAPI(t, root, []string{"cmd", "internal"}, apiAllowed) {
		t.Errorf("%s names %s: only the card-set import's TCGdex fetcher (%s) may name the TCGdex API; the playmat reads card data from published card sets", f, tcgdexAPI, strings.Join(apiAllowed, ", "))
	}
}

// The scanner itself: a hit outside the allow-list is found, a hit in the
// allowed file, in a test file or in another host is not.
func TestNamesAPIScanner(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/cardset/tcgdex.go", `const base = "https://api.tcgdex.net/v2/en"`)
	write("internal/cards/client.go", `const base = "https://api.tcgdex.net/v2/en"`)
	write("internal/cards/client_test.go", `const base = "https://api.tcgdex.net/v2/en"`)
	write("internal/cards/images.go", `const images = "https://assets.tcgdex.net/"`)
	write("cmd/tcgreplay/main.go", `// fetches from api.tcgdex.net`)
	got := namesAPI(t, root, []string{"cmd", "internal"}, []string{"internal/cardset/tcgdex.go"})
	slices.Sort(got)
	if want := []string{"cmd/tcgreplay/main.go", "internal/cards/client.go"}; !slices.Equal(got, want) {
		t.Errorf("hits %v, want %v", got, want)
	}
}
