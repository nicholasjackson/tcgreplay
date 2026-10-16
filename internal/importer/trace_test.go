package importer

import (
	"os"
	"strconv"
	"testing"
)

// TestTrace prints a replay trace for one log: TRACE=<log path> go test
// -run Trace. SEED picks the seed (default 1).
func TestTrace(t *testing.T) {
	path := os.Getenv("TRACE")
	if path == "" {
		t.Skip("set TRACE to a log file")
	}
	entries, _ := loadCorpus(t)
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seed := uint64(1)
	if s := os.Getenv("SEED"); s != "" {
		if seed, err = strconv.ParseUint(s, 10, 64); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Import(string(text), Options{MyDeck: entries, Seed: seed, Trace: os.Stdout})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range res.Report.Problems {
		t.Logf("line %d %q: %s", p.Line, p.Text, p.Err)
	}
}
