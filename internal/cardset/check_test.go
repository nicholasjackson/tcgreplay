package cardset

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cleanDir writes testSet, with an image on every print, as a built set
// that passes every check, and returns its card-set directory.
func cleanDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	s := testSet()
	for i := range s.Prints {
		s.Prints[i].Images = []Image{{Source: "tcgdex", URL: "https://assets.tcgdex.net/x"}}
	}
	if err := Write(filepath.Join(dir, "sets", "tst.json"), s); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeJSON(t *testing.T, path, body string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCleanSet(t *testing.T) {
	dir := cleanDir(t)
	// A correction that resolves and an acknowledged report item are fine.
	writeJSON(t, filepath.Join(dir, "corrections", "tst.json"), `{"corrections":[{"card":"tst-001","path":"/hp","value":70,"reason":"printed"}],
		"acknowledged":[{"card":"tst-002","kind":"disagreement","field":"/hp","reason":"TCGdex is right"}]}`)
	writeJSON(t, filepath.Join(dir, "reports", "tst.json"), `{"set":"tst","summary":{},"items":[
		{"card":"tst-002","kind":"disagreement","field":"/hp","status":"open"},
		{"card":"tst-001","kind":"disagreement","field":"/hp","status":"open"}]}`)
	items, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("clean set: %v", items)
	}
}

func TestCheckFindsOpenItems(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dir string)
		want  []string // each must appear in one item's line
	}{
		{"open report item", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "reports", "tst.json"), `{"set":"tst","summary":{},"items":[{"card":"tst-002","kind":"disagreement","field":"/retreat","status":"open"}]}`)
		}, []string{"tst tst-002 disagreement /retreat"}},
		{"unacknowledged gap", func(t *testing.T, dir string) {
			s, _ := Read(filepath.Join(dir, "sets", "tst.json"))
			p, _ := s.Print("tst-TG01")
			p.Images = nil
			Write(filepath.Join(dir, "sets", "tst.json"), s)
		}, []string{"tst tst-TG01 no-image /images"}},
		{"HTML in text", func(t *testing.T, dir string) {
			s, _ := Read(filepath.Join(dir, "sets", "tst.json"))
			p, _ := s.Print("tst-001")
			p.Text["en"].Attacks[0].Effect = `Discard a <span class="energy-symbol Fire">Fire</span> Energy.`
			Write(filepath.Join(dir, "sets", "tst.json"), s)
		}, []string{"tst tst-001 markup /text/en/attacks/0/effect"}},
		{"missing English name", func(t *testing.T, dir string) {
			b, _ := os.ReadFile(filepath.Join(dir, "sets", "tst.json"))
			os.WriteFile(filepath.Join(dir, "sets", "tst.json"), []byte(strings.Replace(string(b), `"name": "Riolu"`, `"name": ""`, 1)), 0o644)
		}, []string{"tst tst invalid", "no English name"}},
		{"missing English effect", func(t *testing.T, dir string) {
			s, _ := Read(filepath.Join(dir, "sets", "tst.json"))
			p, _ := s.Print("tst-010")
			en := p.Text["en"]
			en.Effect = ""
			p.Text["en"] = en
			Write(filepath.Join(dir, "sets", "tst.json"), s)
		}, []string{"tst tst-010 missing-effect /text/en/effect"}},
		{"dangling correction", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "corrections", "tst.json"), `{"corrections":[
				{"card":"tst-999","path":"/hp","value":1,"reason":"r"},
				{"card":"tst-001","path":"/pairMark","value":true,"reason":"r"}]}`)
		}, []string{"tst tst-999 correction-dangling /hp", "tst tst-001 correction-dangling /pairMark"}},
		{"broken corrections file", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "corrections", "tst.json"), `{"corrections":[{"card":"tst-001","path":"/hp","value":1}]}`)
		}, []string{"tst tst corrections-invalid", "reason"}},
		{"redundant correction still present", func(t *testing.T, dir string) {
			writeJSON(t, filepath.Join(dir, "corrections", "tst.json"), `{"corrections":[{"card":"tst-001","path":"/hp","value":70,"reason":"r"}]}`)
			writeJSON(t, filepath.Join(dir, "reports", "tst.json"), `{"set":"tst","summary":{},"items":[{"card":"tst-001","kind":"correction-redundant","field":"/hp","status":"open"}]}`)
		}, []string{"tst tst-001 correction-redundant /hp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := cleanDir(t)
			tt.setup(t, dir)
			items, err := Check(dir, "tst")
			if err != nil {
				t.Fatal(err)
			}
			var lines []string
			for _, it := range items {
				lines = append(lines, it.String())
			}
			all := strings.Join(lines, "\n")
			for _, w := range tt.want {
				if !strings.Contains(all, w) {
					t.Errorf("check lines lack %q:\n%s", w, all)
				}
			}
		})
	}
}

// A redundant correction that has since been removed, and a report item
// acknowledged after the import, no longer count.
func TestCheckJudgesReportAgainstCurrentCorrections(t *testing.T) {
	dir := cleanDir(t)
	writeJSON(t, filepath.Join(dir, "reports", "tst.json"), `{"set":"tst","summary":{},"items":[
		{"card":"tst-001","kind":"correction-redundant","field":"/hp","status":"open"},
		{"card":"tst-002","kind":"text-changed","field":"/text/en","status":"open"}]}`)
	writeJSON(t, filepath.Join(dir, "corrections", "tst.json"), `{"acknowledged":[{"card":"tst-002","kind":"text-changed","reason":"New wording matches the printed card"}]}`)
	items, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("items %v, want none", items)
	}
}

func TestCheckMissingSet(t *testing.T) {
	if _, err := Check(t.TempDir(), "nope"); err == nil {
		t.Error("checking a set that is not built succeeded")
	}
}
