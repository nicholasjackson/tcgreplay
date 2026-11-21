package design

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

// luminance is a colour's WCAG relative luminance, from #rgb or #rrggbb.
func luminance(hex string) (float64, error) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return 0, fmt.Errorf("%q is not an opaque #rgb or #rrggbb colour", hex)
	}
	var c [3]float64
	for i := range c {
		v, err := strconv.ParseUint(h[2*i:2*i+2], 16, 8)
		if err != nil {
			return 0, fmt.Errorf("%q: %w", hex, err)
		}
		s := float64(v) / 255
		if s <= 0.04045 {
			c[i] = s / 12.92
		} else {
			c[i] = math.Pow((s+0.055)/1.055, 2.4)
		}
	}
	return 0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2], nil
}

// contrast is the WCAG contrast ratio of two colours.
func contrast(a, b string) (float64, error) {
	la, err := luminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := luminance(b)
	if err != nil {
		return 0, err
	}
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05), nil
}

// contrastFailures checks every declared pair in every interface theme and,
// for pairs that touch the mat, under every mat.
func contrastFailures(t Tokens) []string {
	var out []string
	mats := t.Mats.Sets
	if len(mats) == 0 {
		mats = []string{""}
	}
	for _, p := range t.Pairs {
		onMat := strings.HasPrefix(p.Text, "mat-") || strings.HasPrefix(p.Background, "mat-")
		for _, theme := range t.Color.Themes {
			for _, mat := range mats {
				if !onMat && mat != mats[0] {
					continue // not on the mat: one check per theme
				}
				fg, err := t.Resolve(p.Text, theme, mat)
				if err == nil {
					var bg string
					bg, err = t.Resolve(p.Background, theme, mat)
					if err == nil {
						var r float64
						r, err = contrast(fg, bg)
						if err == nil && r < p.Min {
							where := theme
							if onMat {
								where += ", " + mat + " mat"
							}
							out = append(out, fmt.Sprintf("%s (%s) on %s (%s), %s: %.2f:1, needs %.1f:1", p.Text, fg, p.Background, bg, where, r, p.Min))
						}
					}
				}
				if err != nil {
					out = append(out, fmt.Sprintf("%s on %s: %v", p.Text, p.Background, err))
				}
			}
		}
	}
	return out
}

// TestContrast: every text-and-background pairing the tokens declare meets
// 4.5:1 (3:1 for large text, focus rings and markers) in both interface
// themes and under all three mats.
func TestContrast(t *testing.T) {
	tok, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(tok.Pairs) == 0 {
		t.Fatal("tokens.json declares no contrast pairs")
	}
	for _, f := range contrastFailures(tok) {
		t.Error(f)
	}
}

// TestContrastCatchesFailingPair: a planted pair that falls short is
// reported with both colours and where, a passing one is not, and the
// check follows references and repoints mat aliases.
func TestContrastCatchesFailingPair(t *testing.T) {
	tok, err := Parse([]byte(`{"color":{"themes":["light","dark"],"default":"light","tokens":[
		{"name":"ink","value":{"light":"#000000","dark":"#ffffff"}},
		{"name":"panel","value":{"light":"#ffffff","dark":"#000000"}},
		{"name":"faint","value":"#cccccc"},
		{"name":"slate-surface","value":"#222222"},
		{"name":"felt-surface","value":"#dddddd"},
		{"name":"mat-surface","value":"{slate-surface}"},
		{"name":"mat-text","value":"{ink}"}]},
		"mats":{"default":"slate","sets":["slate","felt"]},
		"pairs":[{"text":"ink","background":"panel","min":4.5},
		         {"text":"faint","background":"panel","min":4.5},
		         {"text":"faint","background":"mat-surface","min":4.5}]}`))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(contrastFailures(tok), "\n")
	for _, want := range []string{
		"faint (#cccccc) on panel (#ffffff), light: 1.61:1, needs 4.5:1",
		"faint (#cccccc) on mat-surface (#dddddd), light, felt mat",
		"faint (#cccccc) on mat-surface (#dddddd), dark, felt mat",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing failure %q in:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"ink (", "slate mat"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("reported a passing pair (%q) in:\n%s", unwanted, got)
		}
	}
	// faint on black (dark panel) passes, so the dark panel check is silent.
	if strings.Contains(got, "on panel (#000000)") {
		t.Errorf("reported faint on the dark panel, which passes:\n%s", got)
	}
	if r, _ := contrast("#000000", "#ffffff"); math.Abs(r-21) > 0.01 {
		t.Errorf("black on white is %.2f:1, want 21:1", r)
	}
}
