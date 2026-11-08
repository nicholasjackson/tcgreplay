package cardset

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// DefaultDir is the project's card-set directory.
const DefaultDir = "cardsets"

// pocketSerie is Pokémon TCG Pocket's TCGdex serie: a different game whose
// sets are never built.
const pocketSerie = "tcgp"

// ImportOptions configures an import.
type ImportOptions struct {
	Set     string // TCGdex set id ("me06") or decklist code ("DLR")
	Dir     string // card-set directory; default DefaultDir
	NoMalie bool   // build from TCGdex (and corrections) alone
	TCGdex  *TCGdex
	Malie   *Malie
	Log     io.Writer // progress notes; nil discards them
}

// Summary describes one set's import.
type Summary struct {
	Set     string `json:"set"`
	Prints  int    `json:"prints"`
	Added   int    `json:"added"`
	Changed int    `json:"changed"`
	Gone    int    `json:"gone"`
	Open    int    `json:"open"`
}

// String is the summary line the command prints.
func (s Summary) String() string {
	return fmt.Sprintf("%s: %d prints, %d added, %d changed, %d gone, %d open", s.Set, s.Prints, s.Added, s.Changed, s.Gone, s.Open)
}

// importer holds what one import run fetches once.
type importer struct {
	opts   ImportOptions
	tcgdex *TCGdex
	malie  *Malie
	log    io.Writer

	sets       []TCGdexSetSummary
	details    map[string]TCGdexSet // every set's details, once fetched
	malieIndex map[string]MalieSetRef
}

func newImporter(opts ImportOptions) *importer {
	im := &importer{opts: opts, tcgdex: opts.TCGdex, malie: opts.Malie, log: opts.Log}
	if im.tcgdex == nil {
		im.tcgdex = &TCGdex{}
	}
	if im.malie == nil {
		im.malie = &Malie{}
	}
	if im.log == nil {
		im.log = io.Discard
	}
	if im.opts.Dir == "" {
		im.opts.Dir = DefaultDir
	}
	return im
}

// Import builds one set: it resolves the set, fetches its prints from
// TCGdex and (unless NoMalie) the malie export, merges them and writes the
// built set to <Dir>/sets/<id>.json.
func Import(ctx context.Context, opts ImportOptions) (Summary, error) {
	im := newImporter(opts)
	id, err := im.resolve(ctx, opts.Set)
	if err != nil {
		return Summary{}, err
	}
	return im.importSet(ctx, id)
}

// setList fetches TCGdex's set list once.
func (im *importer) setList(ctx context.Context) ([]TCGdexSetSummary, error) {
	if im.sets == nil {
		sets, err := im.tcgdex.Sets(ctx)
		if err != nil {
			return nil, err
		}
		im.sets = sets
	}
	return im.sets, nil
}

// allDetails fetches every listed set's details once, eight at a time.
func (im *importer) allDetails(ctx context.Context) (map[string]TCGdexSet, error) {
	if im.details != nil {
		return im.details, nil
	}
	sets, err := im.setList(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]TCGdexSet, len(sets))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error
	sem := make(chan struct{}, 8)
	for _, s := range sets {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, err := im.tcgdex.Set(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, err)
				return
			}
			out[id] = d
		}(s.ID)
	}
	wg.Wait()
	if len(errs) > 0 {
		return nil, fmt.Errorf("fetching set details: %w", errors.Join(errs...))
	}
	im.details = out
	return out, nil
}

// detail returns one set's details, from the fetched list if there is one.
func (im *importer) detail(ctx context.Context, id string) (TCGdexSet, error) {
	if d, ok := im.details[id]; ok {
		return d, nil
	}
	return im.tcgdex.Set(ctx, id)
}

// resolve turns a set id or decklist code into a TCGdex set id. An exact
// id (any case) wins; otherwise the set whose decklist code or TCG Online
// code matches, the shorter id winning when several share it.
func (im *importer) resolve(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("no set named")
	}
	sets, err := im.setList(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range sets {
		if s.ID == name {
			return s.ID, nil
		}
	}
	for _, s := range sets {
		if strings.EqualFold(s.ID, name) {
			return s.ID, nil
		}
	}
	details, err := im.allDetails(ctx)
	if err != nil {
		return "", err
	}
	best := ""
	for _, id := range sortedKeys(details) {
		d := details[id]
		if !strings.EqualFold(d.Abbreviation.Official, name) && !strings.EqualFold(d.TCGOnline, name) {
			continue
		}
		if best == "" || len(id) < len(best) || (len(id) == len(best) && id < best) {
			best = id
		}
	}
	if best == "" {
		return "", fmt.Errorf("no TCGdex set with id or code %q: %w", name, ErrNotFound)
	}
	return best, nil
}

// importSet builds the set with the given TCGdex id.
func (im *importer) importSet(ctx context.Context, id string) (Summary, error) {
	set, err := im.detail(ctx, id)
	if err != nil {
		return Summary{}, err
	}
	if set.Serie.ID == pocketSerie {
		return Summary{}, fmt.Errorf("set %s is a Pokémon TCG Pocket set, which is not built", id)
	}
	fmt.Fprintf(im.log, "%s: fetching %d TCGdex cards\n", id, len(set.Cards))
	td, err := im.tcgdexCandidates(ctx, set)
	if err != nil {
		return Summary{}, err
	}
	var ml []candidate
	if !im.opts.NoMalie {
		if ml, err = im.malieCandidates(ctx, set); err != nil {
			return Summary{}, err
		}
	}
	ms := merge(td, ml)
	built := &Set{Format: Format, Set: header(set)}
	for _, m := range ms {
		built.Prints = append(built.Prints, m.Print)
	}
	// Hand corrections apply to every fresh build; the import only reads
	// the corrections file, never writes it.
	corrs, err := ReadCorrections(filepath.Join(im.opts.Dir, "corrections", id+".json"))
	if err != nil {
		return Summary{}, err
	}
	applied, err := applyCorrections(built, corrs.Corrections)
	if err != nil {
		return Summary{}, fmt.Errorf("set %s: %w", id, err)
	}
	// Carry the fresh build over the set already in the project: new
	// prints are added, changed text and art replace the old, prints gone
	// upstream are kept, and other languages' wording is kept.
	path := filepath.Join(im.opts.Dir, "sets", id+".json")
	var prev *Set
	if _, err := os.Stat(path); err == nil {
		if prev, err = Read(path); err != nil {
			return Summary{}, fmt.Errorf("previous build: %w", err)
		}
	}
	cmp := compare(prev, built)
	if err := cmp.set.ValidateShape(); err != nil {
		return Summary{}, fmt.Errorf("set %s: %w", id, err)
	}
	if err := Write(path, cmp.set); err != nil {
		return Summary{}, err
	}
	disagreements := make(map[string][]disagreement, len(ms))
	for _, m := range ms {
		disagreements[m.ID] = m.disagreements
	}
	report := buildReport(reportInput{set: cmp.set, disagreements: disagreements, applied: applied, cmp: cmp, acks: corrs.Acknowledged})
	report.Summary = Summary{Set: id, Prints: len(cmp.set.Prints), Added: len(cmp.added), Changed: len(cmp.changed), Gone: len(cmp.gone), Open: report.open()}
	if err := WriteReport(filepath.Join(im.opts.Dir, "reports", id+".json"), report); err != nil {
		return Summary{}, err
	}
	return report.Summary, nil
}

// header is the built set's header from TCGdex's set details.
func header(set TCGdexSet) SetHeader {
	return SetHeader{
		ID:           set.ID,
		Serie:        set.Serie.ID,
		ReleaseDate:  set.ReleaseDate,
		Abbreviation: strings.TrimSpace(set.Abbreviation.Official),
		TCGOnline:    strings.TrimSpace(set.TCGOnline),
		Text:         map[string]SetText{English: {Name: cleanText(set.Name)}},
	}
}

// tcgdexCandidates fetches and normalises every print the set lists.
func (im *importer) tcgdexCandidates(ctx context.Context, set TCGdexSet) ([]candidate, error) {
	ids := make([]string, 0, len(set.Cards))
	for _, c := range set.Cards {
		ids = append(ids, c.ID)
	}
	bodies, errs := im.tcgdex.Cards(ctx, ids, 8)
	if len(errs) > 0 {
		var all []error
		for _, id := range sortedKeys(errs) {
			all = append(all, errs[id])
		}
		return nil, fmt.Errorf("set %s: fetching cards: %w", set.ID, errors.Join(all...))
	}
	out := make([]candidate, 0, len(ids))
	for _, id := range ids {
		c, err := fromTCGdex(bodies[id])
		if err != nil {
			return nil, fmt.Errorf("card %s: %w", id, err)
		}
		out = append(out, c)
	}
	return out, nil
}

// malieCandidates fetches and normalises the malie set holding the set's
// prints. A set malie does not cover gives none.
func (im *importer) malieCandidates(ctx context.Context, set TCGdexSet) ([]candidate, error) {
	if im.malieIndex == nil {
		idx, err := im.malie.Index(ctx)
		if err != nil {
			return nil, fmt.Errorf("malie index: %w", err)
		}
		im.malieIndex = idx
	}
	key, ref, ok := matchMalieSet(set, im.malieIndex, nil)
	if ok && im.sharesCode(ctx, set) {
		// Another set may share the decklist code: the shorter id keeps
		// it, which needs every set's details.
		details, err := im.allDetails(ctx)
		if err != nil {
			return nil, err
		}
		key, ref, ok = matchMalieSet(set, im.malieIndex, codeOwners(slices.Collect(mapValues(details))))
	}
	if !ok {
		fmt.Fprintf(im.log, "%s: no malie set\n", set.ID)
		return nil, nil
	}
	fmt.Fprintf(im.log, "%s: malie set %s\n", set.ID, key)
	entries, err := im.malie.SetFile(ctx, ref.Path)
	if err != nil {
		return nil, fmt.Errorf("malie set %s: %w", key, err)
	}
	return fromMalie(entries, set), nil
}

// sharesCode reports whether another TCGdex set might share the set's
// decklist code, so the shorter-id rule needs every set's details: true
// when the details are already fetched, or when another listed set's id
// starts with this one's or the other way round (30th and 30th-c, cel25
// and cel25cc), the only way codes have been shared.
func (im *importer) sharesCode(ctx context.Context, set TCGdexSet) bool {
	if im.details != nil {
		return true
	}
	sets, err := im.setList(ctx)
	if err != nil {
		return true
	}
	for _, s := range sets {
		if s.ID != set.ID && (strings.HasPrefix(s.ID, set.ID) || strings.HasPrefix(set.ID, s.ID)) {
			return true
		}
	}
	return false
}

func mapValues[K comparable, V any](m map[K]V) func(func(V) bool) {
	return func(yield func(V) bool) {
		for _, v := range m {
			if !yield(v) {
				return
			}
		}
	}
}

// AllResult is what an all-sets import did.
type AllResult struct {
	Summaries []Summary        // one per set built, in set-id order
	Failed    map[string]error // set id -> why it failed
	Skipped   []string         // Pokémon TCG Pocket sets, never built
	Unmatched []MalieUnmatched // malie sets no TCGdex set claims
}

// MalieUnmatched is a malie set no TCGdex set matches.
type MalieUnmatched struct {
	Key     string // malie set key, "svalt"
	Abbr    string
	Entries int
}

// ImportAll builds every English set TCGdex lists except Pokémon TCG Pocket
// sets, one after another, carrying on past a set that fails. Unless
// NoMalie, it also lists every malie set that no TCGdex set matches, so
// nothing either source lists is silently missed.
func ImportAll(ctx context.Context, opts ImportOptions) (AllResult, error) {
	im := newImporter(opts)
	res := AllResult{Failed: map[string]error{}}
	details, err := im.allDetails(ctx)
	if err != nil {
		return res, err
	}
	claimed := map[string]bool{}
	for _, id := range sortedKeys(details) {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		d := details[id]
		if d.Serie.ID == pocketSerie {
			res.Skipped = append(res.Skipped, id)
			continue
		}
		sum, err := im.importSet(ctx, id)
		if err != nil {
			fmt.Fprintf(im.log, "%s: FAILED: %v\n", id, err)
			res.Failed[id] = err
			continue
		}
		fmt.Fprintf(im.log, "%s\n", sum)
		res.Summaries = append(res.Summaries, sum)
	}
	if opts.NoMalie {
		return res, nil
	}
	if im.malieIndex == nil {
		idx, err := im.malie.Index(ctx)
		if err != nil {
			return res, fmt.Errorf("malie index: %w", err)
		}
		im.malieIndex = idx
	}
	owner := codeOwners(slices.Collect(mapValues(details)))
	for _, id := range sortedKeys(details) {
		if details[id].Serie.ID == pocketSerie {
			continue
		}
		if key, _, ok := matchMalieSet(details[id], im.malieIndex, owner); ok {
			claimed[key] = true
		}
	}
	for _, key := range sortedKeys(im.malieIndex) {
		if !claimed[key] {
			ref := im.malieIndex[key]
			res.Unmatched = append(res.Unmatched, MalieUnmatched{Key: key, Abbr: ref.Abbr, Entries: ref.Num})
		}
	}
	return res, nil
}
