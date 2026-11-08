package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCardSources starts stand-ins for TCGdex and the malie export serving
// a two-print set "tst" (decklist code TST); malie adds a third print.
func fakeCardSources(t *testing.T) (tcgdex, malie string) {
	t.Helper()
	serve := func(bodies map[string]string) string {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, ok := bodies[r.URL.Path]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Write([]byte(b))
		}))
		t.Cleanup(srv.Close)
		return srv.URL
	}
	tcgdex = serve(map[string]string{
		"/sets":          `[{"id":"tst","name":"Test Set"}]`,
		"/sets/tst":      `{"id":"tst","name":"Test Set","serie":{"id":"me"},"abbreviation":{"official":"TST"},"cards":[{"id":"tst-001","localId":"001"},{"id":"tst-002","localId":"002"}]}`,
		"/cards/tst-001": `{"id":"tst-001","localId":"001","name":"Riolu","category":"Pokemon","hp":70,"types":["Fighting"],"stage":"Basic","attacks":[{"cost":["Fighting"],"name":"Punch","damage":30}]}`,
		"/cards/tst-002": `{"id":"tst-002","localId":"002","name":"Aarune","category":"Trainer","trainerType":"Supporter"}`,
	})
	malie = serve(map[string]string{
		"/index.json": `{"en-US":{"tst1":{"path":"tst1.json","abbr":"TST"}}}`,
		"/tst1.json": `[{"name":"Aarune","card_type":"TRAINER","subtype":"SUPPORTER","text":[{"kind":"TEXT_BOX","text":"Search your deck."}],"collector_number":{"numeric":2},"ext":{"tcgl":{"cardID":"tst1_2","longFormID":"Aarune_tst1_2_std"}}},
			{"name":"Lucario","card_type":"POKEMON","stage":"STAGE1","stage_text":"Evolves from Riolu","hp":130,"text":[{"kind":"ATTACK","name":"Aura Sphere","cost":["FIGHTING"],"damage":{"amount":130}}],"collector_number":{"numeric":3},"ext":{"tcgl":{"cardID":"tst1_3","longFormID":"Lucario_tst1_3_std"}}}]`,
	})
	return tcgdex, malie
}

func TestSetsImport(t *testing.T) {
	tcgdex, malie := fakeCardSources(t)
	dir := filepath.Join(t.TempDir(), "cardsets")
	path := filepath.Join(dir, "sets", "tst.json")
	src := []string{"-tcgdex-base", tcgdex, "-malie-base", malie}

	// By decklist code, with flags after the set.
	code, stdout, stderr := runMain(t, append([]string{"sets", "import", "TST", "-dir", dir}, src...)...)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "tst: 3 prints") {
		t.Errorf("stdout %q lacks the summary", stdout)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"id": "tst-001"`, `"id": "tst-003"`, `"effect": "Search your deck."`, `"name": "Test Set"`, `"category": "Trainer"`} {
		if !bytes.Contains(first, []byte(want)) {
			t.Errorf("built set lacks %s", want)
		}
	}

	// By id, again: identical bytes.
	if code, _, stderr := runMain(t, append([]string{"sets", "import", "-dir", dir, "tst"}, src...)...); code != 0 {
		t.Fatalf("re-import exit %d; stderr:\n%s", code, stderr)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Error("re-importing unchanged sources changed the bytes")
	}

	// Without malie: TCGdex's two prints only.
	noMalie := filepath.Join(t.TempDir(), "cardsets")
	code, stdout, stderr = runMain(t, append([]string{"sets", "import", "tst", "-no-malie", "-dir", noMalie}, src...)...)
	if code != 0 {
		t.Fatalf("-no-malie exit %d; stderr:\n%s", code, stderr)
	}
	if !strings.Contains(stdout, "tst: 2 prints") {
		t.Errorf("-no-malie stdout %q, want 2 prints", stdout)
	}
	b, _ := os.ReadFile(filepath.Join(noMalie, "sets", "tst.json"))
	if bytes.Contains(b, []byte("malie")) {
		t.Error("-no-malie build names malie")
	}
}

func TestSetsUsageErrors(t *testing.T) {
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"sets"}, 2, "usage:"},
		{[]string{"sets", "frobnicate"}, 2, `unknown sets command "frobnicate"`},
		{[]string{"sets", "import"}, 2, "exactly one set"},
		{[]string{"sets", "import", "a", "b"}, 2, "exactly one set"},
		{[]string{"sets", "import", "nope", "-tcgdex-base", "http://127.0.0.1:1", "-dir", "x"}, 1, "127.0.0.1:1"},
	}
	for _, tt := range tests {
		code, _, stderr := runMain(t, tt.args...)
		if code != tt.code || !strings.Contains(stderr, tt.want) {
			t.Errorf("%v: exit %d, stderr %q; want exit %d and %q", tt.args, code, stderr, tt.code, tt.want)
		}
	}
}

// Card-set data is published from the repository and never goes into the
// container image (or the binary, which embeds nothing outside its own
// packages).
func TestCardSetsStayOutOfImage(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".dockerignore"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "cardsets" {
			return
		}
	}
	t.Error(".dockerignore does not exclude cardsets")
}

// sets check exits 0 on a clean set and 1, naming the item, on an open one.
func TestSetsCheck(t *testing.T) {
	tcgdex, malie := fakeCardSources(t)
	dir := filepath.Join(t.TempDir(), "cardsets")
	if code, _, stderr := runMain(t, "sets", "import", "tst", "-dir", dir, "-tcgdex-base", tcgdex, "-malie-base", malie); code != 0 {
		t.Fatalf("import exit %d; stderr:\n%s", code, stderr)
	}
	// The fake set has no images: three open no-image items.
	code, stdout, _ := runMain(t, "sets", "check", "-dir", dir)
	if code != 1 || !strings.Contains(stdout, "tst tst-001 no-image /images") {
		t.Errorf("open set: exit %d, stdout %q; want 1 naming the item", code, stdout)
	}
	os.MkdirAll(filepath.Join(dir, "corrections"), 0o755)
	os.WriteFile(filepath.Join(dir, "corrections", "tst.json"), []byte(`{"acknowledged":[
		{"card":"tst-001","kind":"no-image","reason":"no art yet"},
		{"card":"tst-002","kind":"no-image","reason":"no art yet"},
		{"card":"tst-003","kind":"no-image","reason":"no art yet"}]}`), 0o644)
	code, stdout, stderr := runMain(t, "sets", "check", "tst", "-dir", dir)
	if code != 0 {
		t.Errorf("clean set: exit %d, stdout %q, stderr %q; want 0", code, stdout, stderr)
	}
	if code, _, stderr := runMain(t, "sets", "check", "nope", "-dir", dir); code != 1 || !strings.Contains(stderr, "nope") {
		t.Errorf("missing set: exit %d, stderr %q", code, stderr)
	}
}

// sets import -all builds every set and prints a summary line per set.
func TestSetsImportAll(t *testing.T) {
	tcgdex, malie := fakeCardSources(t)
	dir := filepath.Join(t.TempDir(), "cardsets")
	code, stdout, stderr := runMain(t, "sets", "import", "-all", "-dir", dir, "-tcgdex-base", tcgdex, "-malie-base", malie)
	if code != 0 {
		t.Fatalf("exit %d; stderr:\n%s", code, stderr)
	}
	for _, want := range []string{"tst: 3 prints", "1 sets built", "0 failed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	if code, _, stderr := runMain(t, "sets", "import", "-all", "tst", "-dir", dir); code != 2 || !strings.Contains(stderr, "or -all") {
		t.Errorf("-all with a set: exit %d, stderr %q; want a usage error", code, stderr)
	}
}
