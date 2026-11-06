package importer

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nicholasjackson/tcgreplay/internal/state"
)

func TestCodeFor(t *testing.T) {
	codes := lineCodes("NicJackson evolved (me1_76) Riolu to (mebsp_33) Mega Lucario ex on the Bench.\n" +
		"   • (me1_179) Mega Lucario ex, (me1_119) Judge, (me1_188) Mega Lucario ex\n" +
		"   • (me1_179) Mega Lucario ex, (me1_179) Mega Lucario ex\n" +
		"No codes here.")
	for _, tc := range []struct {
		line       int
		name, want string
	}{
		{1, "Riolu", "me1_76"},
		{1, "Mega Lucario ex", "mebsp_33"},
		{2, "Judge", "me1_119"},
		{2, "Mega Lucario ex", ""}, // two prints on one line: no telling which is which
		{3, "Mega Lucario ex", "me1_179"},
		{4, "Riolu", ""},
		{1, "Solrock", ""},
	} {
		if got := codeFor(codes[tc.line], tc.name); got != tc.want {
			t.Errorf("line %d %s: code %q, want %q", tc.line, tc.name, got, tc.want)
		}
	}
	if lineCodes("Setup\nNicJackson drew 7 cards.") != nil {
		t.Error("a log without codes has line codes")
	}
}

// TestLogCodesPickEachCopysPrint gives each copy of a card the print the
// log names for it, so alternate arts show as themselves: the corpus game
// with TCG Live's card codes added the way newer logs carry them. The
// opening hand names two prints of Mega Lucario ex on one line, so those
// copies get their print from the later lines that name one alone.
func TestLogCodesPickEachCopysPrint(t *testing.T) {
	entries, _ := loadCorpus(t)
	raw, err := os.ReadFile(filepath.Join(corpus, "logs/2026-10-11_gatorusoju_mewex-bronzong-lopunny.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	tag := func(no int, old, new string) {
		t.Helper()
		if !strings.Contains(lines[no-1], old) {
			t.Fatalf("line %d %q lacks %q", no, lines[no-1], old)
		}
		lines[no-1] = strings.Replace(lines[no-1], old, new, 1)
	}
	tag(7, "• Mega Lucario ex, Judge, Mega Lucario ex,", "• (me1_179) Mega Lucario ex, (me1_119) Judge, (mebsp_33) Mega Lucario ex,")
	tag(10, "played Solrock", "played (me1_75) Solrock")
	tag(202, "to Mega Lucario ex", "to (me1_188) Mega Lucario ex")
	tag(211, "drew Mega Lucario ex", "drew (mebsp_33) Mega Lucario ex")
	text := strings.Join(lines, "\n")

	var asked []string
	defOfCode := func(name, code string) string {
		asked = append(asked, name+"="+code)
		if code == "me1_75" {
			return "" // a code that names no card of that name: keep the list's print
		}
		return "print:" + code
	}
	defs := func(opts Options) map[string][]string {
		t.Helper()
		res, err := Import(text, opts)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range res.Report.Problems {
			t.Errorf("line %d %q: %s", p.Line, p.Text, p.Err)
		}
		st, err := state.Fold(res.Events())
		if err != nil {
			t.Fatal(err)
		}
		out := map[string][]string{}
		for _, c := range st.Cards {
			if c.Owner == MeSeat {
				out[c.Name] = append(out[c.Name], c.Def)
			}
		}
		for _, d := range out {
			slices.Sort(d)
		}
		return out
	}

	got := defs(Options{MyDeck: entries, Seed: 1, DefOfCode: defOfCode})
	// Three copies: the evolved one is the Mega Hyper Rare, the drawn one
	// the promo, and the third, never named alone, keeps the list's print.
	if want := []string{"", "print:me1_188", "print:mebsp_33"}; !slices.Equal(got["Mega Lucario ex"], want) {
		t.Errorf("Mega Lucario ex prints %q, want %q", got["Mega Lucario ex"], want)
	}
	if !slices.Contains(asked, "Solrock=me1_75") || slices.Contains(got["Solrock"], "print:me1_75") {
		t.Errorf("Solrock prints %q (asked %q): a rejected code must keep the list's print", got["Solrock"], asked)
	}
	// Judge is the only one of its name on the opening hand's line, so the
	// copy named there gets its code even in a list.
	if !slices.Contains(got["Judge"], "print:me1_119") {
		t.Errorf("Judge prints %q, want one print:me1_119", got["Judge"])
	}

	// Without the lookup, every copy keeps the list's print, as before.
	plain := defs(Options{MyDeck: entries, Seed: 1})
	if want := []string{"", "", ""}; !slices.Equal(plain["Mega Lucario ex"], want) {
		t.Errorf("without DefOfCode, Mega Lucario ex prints %q, want the list's %q", plain["Mega Lucario ex"], want)
	}
}
