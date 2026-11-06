// Package cards looks up card definitions from TCGdex and caches them on
// disk, so card text is quoted from data rather than memory.
package cards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrNotFound means TCGdex has no such card or set.
var ErrNotFound = errors.New("not found")

// Ability is a Pokémon's ability.
type Ability struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Effect string `json:"effect"`
}

// Attack is a Pokémon's attack. Damage is a string because TCGdex mixes
// numbers and values like "30+".
type Attack struct {
	Cost   []string `json:"cost,omitempty"`
	Name   string   `json:"name"`
	Effect string   `json:"effect,omitempty"`
	Damage Damage   `json:"damage,omitempty"`
}

// Damage decodes a number or a string.
type Damage string

func (d *Damage) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*d = Damage(n.String())
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*d = Damage(s)
	return nil
}

// TypeValue is a weakness or resistance.
type TypeValue struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// SetRef is the set a card belongs to.
type SetRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Card is a card definition.
type Card struct {
	ID             string      `json:"id"`
	LocalID        string      `json:"localId"`
	Name           string      `json:"name"`
	Category       string      `json:"category"` // Pokemon, Trainer, Energy
	Image          string      `json:"image,omitempty"`
	Set            SetRef      `json:"set"`
	HP             int         `json:"hp,omitempty"`
	Types          []string    `json:"types,omitempty"`
	Stage          string      `json:"stage,omitempty"`
	EvolveFrom     string      `json:"evolveFrom,omitempty"`
	Suffix         string      `json:"suffix,omitempty"`
	Abilities      []Ability   `json:"abilities,omitempty"`
	Attacks        []Attack    `json:"attacks,omitempty"`
	Weaknesses     []TypeValue `json:"weaknesses,omitempty"`
	Resistances    []TypeValue `json:"resistances,omitempty"`
	Retreat        int         `json:"retreat,omitempty"`
	TrainerType    string      `json:"trainerType,omitempty"` // Item, Supporter, Stadium, Tool
	EnergyType     string      `json:"energyType,omitempty"`  // Normal, Special
	Effect         string      `json:"effect,omitempty"`
	RegulationMark string      `json:"regulationMark,omitempty"`
}

// IsPokemon reports whether the card is a Pokémon.
func (c *Card) IsPokemon() bool { return c.Category == "Pokemon" }

// IsBasic reports whether the card is a Basic Pokémon.
func (c *Card) IsBasic() bool { return c.IsPokemon() && c.Stage == "Basic" }

// Text renders the card as plain text for quoting.
func (c *Card) Text() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s)", c.Name, c.ID)
	switch c.Category {
	case "Pokemon":
		fmt.Fprintf(&b, "\n%s Pokémon", c.Stage)
		if c.Suffix != "" {
			fmt.Fprintf(&b, " %s", c.Suffix)
		}
		if c.EvolveFrom != "" {
			fmt.Fprintf(&b, ", evolves from %s", c.EvolveFrom)
		}
		fmt.Fprintf(&b, ". HP %d. Type %s.", c.HP, strings.Join(c.Types, "/"))
		for _, a := range c.Abilities {
			fmt.Fprintf(&b, "\n%s: %s. %s", a.Type, a.Name, a.Effect)
		}
		for _, a := range c.Attacks {
			fmt.Fprintf(&b, "\nAttack [%s] %s", strings.Join(a.Cost, ","), a.Name)
			if a.Damage != "" {
				fmt.Fprintf(&b, " %s", a.Damage)
			}
			if a.Effect != "" {
				fmt.Fprintf(&b, ". %s", a.Effect)
			}
		}
		for _, w := range c.Weaknesses {
			fmt.Fprintf(&b, "\nWeakness %s %s", w.Type, w.Value)
		}
		for _, r := range c.Resistances {
			fmt.Fprintf(&b, "\nResistance %s %s", r.Type, r.Value)
		}
		fmt.Fprintf(&b, "\nRetreat %d", c.Retreat)
	case "Trainer":
		fmt.Fprintf(&b, "\nTrainer: %s", c.TrainerType)
		if c.Effect != "" {
			fmt.Fprintf(&b, "\n%s", c.Effect)
		}
	case "Energy":
		fmt.Fprintf(&b, "\n%s Energy", c.EnergyType)
		if c.Effect != "" {
			fmt.Fprintf(&b, "\n%s", c.Effect)
		}
	}
	if c.RegulationMark != "" {
		fmt.Fprintf(&b, "\nRegulation mark %s", c.RegulationMark)
	}
	return b.String()
}

// DB looks up card definitions.
type DB interface {
	// Card returns a card by TCGdex id.
	Card(ctx context.Context, id string) (*Card, error)
	// Cached returns a card only if it is already cached; it never blocks on
	// the network.
	Cached(id string) *Card
	// Resolve maps a set code as printed in TCG Live exports (e.g. "SVI")
	// and a collector number to a TCGdex card id.
	Resolve(ctx context.Context, setCode, number string) (string, error)
	// ByName picks a TCGdex card id for a card known only by name. moves,
	// the attacks and Abilities the card was seen using, pick among its
	// prints: same-named cards can be different Pokémon.
	ByName(ctx context.Context, name string, moves ...string) (string, error)
}

// Client is a TCGdex client with a disk cache.
type Client struct {
	Base     string // default https://api.tcgdex.net/v2/en
	CacheDir string
	HTTP     *http.Client
	// Offline disables network access; only cached data is used.
	Offline bool
	// Store holds the card source's responses. NewClient sets a disk
	// store under CacheDir; a nil Store means the same.
	Store Store
	// Timeout bounds each request to the card source. NewClient sets 5
	// seconds.
	Timeout time.Duration
	// Now is the clock that stamps fetched entries. NewClient sets
	// time.Now; a nil Now means the same.
	Now func() time.Time

	// mu guards the in-memory maps and the catalog pointer only. It is
	// never held across disk or network I/O, so a lookup of a cached card
	// never waits behind a fetch or a catalog build.
	mu       sync.Mutex
	mem      map[string]cardEntry // card id -> card and when it was fetched
	cat      *catalog             // set data and indexes; nil until first used
	catMu    sync.Mutex           // serialises catalog updates; held for CPU work only
	sweeping bool                 // a background sweep of the catalog is running
	flight   flightGroup          // one source request (or catalog build) per key at a time
	tracker  refreshTracker       // recent failures and refreshes
	bgwg     sync.WaitGroup       // background refreshes in flight
	// gen is the clear generation: Clear moves it on. A load remembers the
	// generation it started under and keeps its result in memory (or
	// publishes its catalog) only if no clear happened meanwhile, so a
	// fetch begun before a clear never puts pre-clear data back.
	gen uint64
}

// NewClient returns a client caching under dir.
func NewClient(dir string) *Client {
	return &Client{
		Base:     "https://api.tcgdex.net/v2/en",
		CacheDir: dir,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		Store:    NewDiskStore(dir),
		Timeout:  5 * time.Second,
		Now:      time.Now,
		mem:      map[string]cardEntry{},
	}
}

// cardEntry is a card held in memory with the time it was fetched.
type cardEntry struct {
	card    *Card
	fetched time.Time
	// cleared marks a card a clear covered. Card treats it as a miss and
	// fetches it again, keeping this copy only if the card source fails;
	// Cached still returns it, so open games keep their cards, and warms
	// it in the background.
	cleared bool
}

func (c *Client) store() Store {
	if c.Store == nil {
		return NewDiskStore(c.CacheDir)
	}
	return c.Store
}

// localStore is the store's local tier: the store itself, unless it puts
// a local tier in front of a shared one (see NewTieredStore). Cached runs
// under the game lock and offline mode must never touch the network, so
// both read only this tier.
func (c *Client) localStore() Store {
	s := c.store()
	if t, ok := s.(interface{ Local() Store }); ok {
		return t.Local()
	}
	return s
}

// readStore is the store a lookup reads before asking the card source:
// the whole store online, the local tier alone offline.
func (c *Client) readStore() Store {
	if c.Offline {
		return c.localStore()
	}
	return c.store()
}

func (c *Client) timeout() time.Duration {
	if c.Timeout <= 0 {
		return 5 * time.Second
	}
	return c.Timeout
}

func (c *Client) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}
	return c.Now()
}

// remember keeps a decoded card in memory, unless memory already holds a
// copy fetched later or a clear happened since the load began under gen,
// and returns the copy memory holds (the decoded card itself when a clear
// intervened). A cleared copy always gives way to a fetched one.
func (c *Client) remember(id string, card *Card, fetched time.Time, gen uint64) cardEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return cardEntry{card: card, fetched: fetched}
	}
	if c.mem == nil {
		c.mem = map[string]cardEntry{}
	}
	if old, ok := c.mem[id]; ok && !old.cleared && old.fetched.After(fetched) {
		return old
	}
	c.mem[id] = cardEntry{card: card, fetched: fetched}
	return c.mem[id]
}

// generation returns the current clear generation.
func (c *Client) generation() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.gen
}

// DefaultCacheDir returns $XDG_CACHE_HOME/tcgreplay or ~/.cache/tcgreplay.
func DefaultCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "tcgreplay")
	}
	return filepath.Join(os.TempDir(), "tcgreplay-cache")
}

func safeName(id string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(id)
}

// loadEntry is the one path every lookup loads a card source response
// through: the store, then the card source. With out set, the body is
// decoded into it.
func (c *Client) loadEntry(ctx context.Context, key string, out any) (Entry, error) {
	e, err := c.readStore().Get(ctx, key)
	if err != nil {
		if c.Offline {
			return Entry{}, fmt.Errorf("%s: not cached and offline: %w", key, ErrNotFound)
		}
		if e, err = c.shared(ctx, key, true); err != nil {
			return Entry{}, err
		}
	}
	return e, decodeEntry(key, e, out)
}

// fetchEntry fetches key from the card source, whatever the store holds,
// and stores it with its fetch time.
func (c *Client) fetchEntry(ctx context.Context, key string, out any) (Entry, error) {
	e, err := c.shared(ctx, key, false)
	if err != nil {
		return Entry{}, err
	}
	return e, decodeEntry(key, e, out)
}

func decodeEntry(key string, e Entry, out any) error {
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(e.Data, out); err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return nil
}

// shared fetches key from the card source through the single-flight
// group, so simultaneous requests for one key make one request. The fetch
// is bounded by the client's timeout and runs on a context of its own, so
// a caller giving up does not fail the others. A body that does not decode
// as its kind is refused, and a good one is stored. With recheck, an entry
// another caller stored meanwhile is used instead of fetching again. The
// outcome of fetching a card is remembered (see recentlyFailed).
func (c *Client) shared(ctx context.Context, key string, recheck bool) (Entry, error) {
	// A lookup after a clear never joins a fetch begun before it.
	fkey := fmt.Sprintf("%s#%d", key, c.generation())
	v, err := c.flight.do(ctx, fkey, c.timeout(), func(fctx context.Context) (any, error) {
		if recheck {
			// Another caller's write lands in the local tier first, so
			// that is all the recheck reads: the caller has just missed
			// in the shared tier.
			if e, err := c.localStore().Get(fctx, key); err == nil {
				return e, nil
			}
		}
		e, err := c.fetch(fctx, key)
		if err == nil {
			err = validateEntry(key, e.Data)
		}
		if id, ok := strings.CutPrefix(key, "cards/"); ok {
			c.noteCard(id, err)
		}
		if err != nil {
			return Entry{}, err
		}
		c.store().Put(fctx, key, e) // a cache write failure only costs a refetch
		return e, nil
	})
	if err != nil {
		return Entry{}, err
	}
	return v.(Entry), nil
}

// validateEntry checks a fetched body decodes as the kind its key names.
func validateEntry(key string, b []byte) error {
	var out any
	switch {
	case key == "sets":
		out = &[]setSummary{}
	case strings.HasPrefix(key, "sets/"):
		out = &setDetail{}
	case strings.HasPrefix(key, "cards/"):
		out = &Card{}
	default:
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("tcgdex %s: %w", key, err)
	}
	return nil
}

// sourcePath maps a key to its card source path: "sets" is /sets and
// "<kind>/<id>" is /<kind>/<escaped id>.
func sourcePath(key string) string {
	kind, id, ok := strings.Cut(key, "/")
	if !ok {
		return "/" + key
	}
	return "/" + kind + "/" + url.PathEscape(id)
}

// fetch requests key from the card source and stamps it with the time it
// answered.
func (c *Client) fetch(ctx context.Context, key string) (Entry, error) {
	path := sourcePath(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return Entry{}, err
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return Entry{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Entry{}, fmt.Errorf("tcgdex %s: %w", path, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return Entry{}, fmt.Errorf("tcgdex %s: %s", path, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return Entry{}, err
	}
	if !json.Valid(b) {
		return Entry{}, fmt.Errorf("tcgdex %s: response is not JSON", path)
	}
	return Entry{Data: b, Fetched: c.now()}, nil
}

// Card implements DB. A card the card source failed to return (unknown,
// error or timeout) is not asked for again for RetryAfter: lookups in
// between fail at once.
func (c *Client) Card(ctx context.Context, id string) (*Card, error) {
	if id == "" {
		return nil, fmt.Errorf("empty card id: %w", ErrNotFound)
	}
	c.mu.Lock()
	gen := c.gen
	old, ok := c.mem[id]
	c.mu.Unlock()
	if ok && (!old.cleared || c.Offline) {
		c.refreshCardIfStale(id, old)
		return old.card, nil
	}
	// A miss, or a card a clear covered: the store, then the card source.
	// A cleared card's old copy stands in if neither can answer.
	card, err := c.loadCard(ctx, id, gen)
	if err != nil {
		if ok {
			return old.card, nil
		}
		return nil, err
	}
	return card, nil
}

// loadCard loads card id from the store or else the card source, keeps it
// in memory (subject to the clear generation gen) and returns it.
func (c *Client) loadCard(ctx context.Context, id string, gen uint64) (*Card, error) {
	key := "cards/" + id
	e, err := c.readStore().Get(ctx, key)
	if err != nil {
		switch {
		case c.Offline:
			return nil, fmt.Errorf("%s: not cached and offline: %w", key, ErrNotFound)
		case c.recentlyFailed(id):
			return nil, fmt.Errorf("%s: recently failed: %w", id, ErrNotFound)
		}
		if e, err = c.shared(ctx, key, true); err != nil {
			return nil, err
		}
	}
	var card Card
	if err := decodeEntry(key, e, &card); err != nil {
		return nil, err
	}
	energyArt(&card)
	ce := c.remember(id, &card, e.Fetched, gen)
	c.refreshCardIfStale(id, ce)
	return ce.card, nil
}

// Cached implements DB. It reads memory and the local tier only, never the
// network: a card held only in a shared store is nil here until a Card
// lookup brings it to local disk.
func (c *Client) Cached(id string) *Card {
	c.mu.Lock()
	gen := c.gen
	e, ok := c.mem[id]
	c.mu.Unlock()
	if ok {
		if e.cleared {
			c.warmCleared(id)
		} else {
			c.refreshCardIfStale(id, e)
		}
		return e.card
	}
	se, err := c.localStore().Get(context.Background(), "cards/"+id)
	if err != nil {
		return nil
	}
	var cc Card
	if json.Unmarshal(se.Data, &cc) != nil {
		return nil
	}
	energyArt(&cc)
	ce := c.remember(id, &cc, se.Fetched, gen)
	c.refreshCardIfStale(id, ce)
	return ce.card
}

// basicEnergyArt is Crown Zenith's Basic Energy art. TCGdex has no images
// for the Scarlet & Violet and Mega Evolution Energy prints that decks
// use, and every other print with art is a gold hyper rare.
var basicEnergyArt = map[string]string{
	"Grass": "152", "Fire": "153", "Water": "154", "Lightning": "155",
	"Psychic": "156", "Fighting": "157", "Darkness": "158", "Metal": "159",
}

// energyArt gives a Basic Energy card without an image the art of the
// same type. Only the picture is borrowed; the card keeps its identity.
func energyArt(c *Card) {
	if c.Image != "" || c.Category != "Energy" || c.EnergyType != "Normal" {
		return
	}
	for t, n := range basicEnergyArt {
		if strings.Contains(c.Name, t) {
			c.Image = "https://assets.tcgdex.net/en/swsh/swsh12.5/" + n
			return
		}
	}
}

type setSummary struct {
	ID string `json:"id"`
}

type setDetail struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ReleaseDate string `json:"releaseDate"`
	Serie       struct {
		ID string `json:"id"`
	} `json:"serie"`
	Abbreviation struct {
		Official string `json:"official"`
	} `json:"abbreviation"`
	TCGOnline string `json:"tcgOnline"`
	Cards     []struct {
		ID      string `json:"id"`
		LocalID string `json:"localId"`
		Name    string `json:"name"`
		Image   string `json:"image"`
	} `json:"cards"`
}

// codeAliases maps TCG Live set codes that differ from TCGdex
// abbreviations.
var codeAliases = map[string]string{
	"PR-SV": "SVP",
	"PR-SW": "SWSHP",
}

// setAliases maps set codes straight to a TCGdex set id, for sets whose
// official abbreviation is shared (30th Celebration and its Classic
// Collection are both "30C") or that exports spell their own way.
var setAliases = map[string]string{
	"30TH": "30th",
	"M30":  "30th",
}

// logCodePrefix maps the set part of a TCG Live log code that does not
// follow the series+number pattern to a TCGdex set id.
var logCodePrefix = map[string]string{
	"mebsp": "mep",
	"svbsp": "svp",
}

var logCodeRe = regexp.MustCompile(`^([a-z]+?)(\d+)?(?:-(\d+))?_(\d+)(?:_[a-z0-9]+)*$`)

// LogCodeID turns a TCG Live log card code into the TCGdex id it most
// likely names: me3_121 is me03-121, me2-5_172 me02.5-172, zsv10-5_79
// sv10.5b-079 (Black Bolt), mebsp_33 mep-033 and swsh1_213 swsh1-213. Callers should check the
// card exists and has the expected name.
func LogCodeID(code string) (string, bool) {
	m := logCodeRe.FindStringSubmatch(strings.ToLower(code))
	if m == nil {
		return "", false
	}
	prefix, major, half, num := m[1], m[2], m[3], m[4]
	n, _ := strconv.Atoi(num)
	var set string
	switch {
	case major == "":
		set = prefix
		if s, ok := logCodePrefix[prefix]; ok {
			set = s
		}
	default:
		suffix := ""
		switch prefix {
		case "zsv": // Zekrom: Black Bolt
			prefix, suffix = "sv", "b"
		case "rsv": // Reshiram: White Flare
			prefix, suffix = "sv", "w"
		}
		mj, _ := strconv.Atoi(major)
		// TCGdex pads the set number for Scarlet & Violet and Mega
		// Evolution (sv01, me01) but not for older series (swsh1).
		if prefix == "sv" || prefix == "me" {
			set = fmt.Sprintf("%s%02d", prefix, mj)
		} else {
			set = fmt.Sprintf("%s%d", prefix, mj)
		}
		if half != "" {
			set += "." + half
		}
		set += suffix
	}
	return fmt.Sprintf("%s-%03d", set, n), true
}

func (c *Client) set(ctx context.Context, id string) (*setDetail, error) {
	s, _, err := c.setFetched(ctx, id)
	return s, err
}

// setFetched loads a set's card list with the time it was fetched.
func (c *Client) setFetched(ctx context.Context, id string) (*setDetail, time.Time, error) {
	var s setDetail
	e, err := c.loadEntry(ctx, "sets/"+id, &s)
	return &s, e.Fetched, err
}

// Resolve implements DB.
func (c *Client) Resolve(ctx context.Context, setCode, number string) (string, error) {
	code := normCode(setCode)
	var cat *catalog
	setID, ok := setAliases[code]
	if ok {
		c.mu.Lock()
		cat = c.cat // an alias needs no catalog, but uses one already built
		c.mu.Unlock()
	} else {
		var err error
		if cat, err = c.ensureCatalog(ctx); err != nil {
			return "", err
		}
		setID, ok = cat.setIndex[code]
	}
	if !ok && c.missAllowed("code:"+code) {
		// The cached set list may predate a new set.
		if fresh, err := c.refreshSetList(ctx, cat); err == nil && fresh != nil {
			cat = fresh
			setID, ok = cat.setIndex[code]
		}
	}
	if !ok {
		return "", fmt.Errorf("unknown set code %q: %w", setCode, ErrNotFound)
	}
	var s *setDetail
	if e, ok := cat.setEntry(setID); ok {
		s = e.d
	} else {
		var err error
		if s, err = c.set(ctx, setID); err != nil {
			return "", err
		}
	}
	if id, ok := findNumber(s, number); ok {
		return id, nil
	}
	if c.missAllowed("num:" + setID + ":" + strings.ToUpper(number)) {
		// The cached card list may predate a card added to the set.
		if _, fresh, err := c.refreshSet(ctx, setID); err == nil {
			if id, ok := findNumber(fresh, number); ok {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("no card %s %s in set %s: %w", setCode, number, setID, ErrNotFound)
}

// normCode normalises a set code as decklists print it: upper case, with
// TCG Live's own codes mapped to TCGdex abbreviations (codeAliases). The
// result is looked up in setAliases, then the catalog's code index.
func normCode(setCode string) string {
	code := strings.ToUpper(strings.TrimSpace(setCode))
	if a, ok := codeAliases[code]; ok {
		code = a
	}
	return code
}

// findNumber finds a collector number in a set's card list: exactly, or
// as the same number without leading zeros.
func findNumber(s *setDetail, number string) (string, bool) {
	want, numErr := strconv.Atoi(number)
	for _, card := range s.Cards {
		if strings.EqualFold(card.LocalID, number) {
			return card.ID, true
		}
		if numErr == nil {
			if n, err := strconv.Atoi(card.LocalID); err == nil && n == want {
				return card.ID, true
			}
		}
	}
	return "", false
}

// nameKey normalises a card name for lookup: case and apostrophe style
// differ between TCG Live and TCGdex.
func nameKey(name string) string {
	name = strings.NewReplacer("’", "'", "‘", "'").Replace(name)
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// SameName reports whether two card names match, ignoring case, spacing
// and apostrophe style.
func SameName(a, b string) bool { return nameKey(a) == nameKey(b) }

// ByName implements DB. A name has many prints; it picks the newest one
// with art (what Standard play uses), preferring the regular print over
// higher-numbered alternate arts. Pokémon TCG Pocket sets are skipped. "Basic Fire Energy", as logs name it,
// also matches "Fire Energy". With moves, it picks the preferred print
// that has the most of them: Chi-Yu seen using Ground Melter is the
// Twilight Masquerade Fire Chi-Yu, not a newer Darkness one.
func (c *Client) ByName(ctx context.Context, name string, moves ...string) (string, error) {
	ids, err := c.prints(ctx, name)
	if err != nil {
		return "", err
	}
	if len(moves) == 0 {
		return ids[0], nil
	}
	best, most := ids[0], 0
	for _, id := range ids {
		card, err := c.Card(ctx, id)
		if err != nil {
			continue // offline or gone: not a candidate
		}
		if n := card.hasMoves(moves); n > most {
			best, most = id, n
			if n == len(moves) {
				break
			}
		}
	}
	return best, nil
}

// prints lists the prints of name, preferred first.
func (c *Client) prints(ctx context.Context, name string) ([]string, error) {
	cat, err := c.ensureCatalog(ctx)
	if err != nil {
		return nil, err
	}
	key := nameKey(name)
	if ids, ok := cat.byName[key]; ok {
		return ids, nil
	}
	if k, ok := strings.CutPrefix(key, "basic "); ok {
		if ids, ok := cat.byName[k]; ok {
			return ids, nil
		}
	}
	return nil, fmt.Errorf("no card named %q: %w", name, ErrNotFound)
}

// hasMoves counts the moves the card has as an attack or Ability.
func (c *Card) hasMoves(moves []string) int {
	n := 0
	for _, m := range moves {
		if slices.ContainsFunc(c.Attacks, func(a Attack) bool { return a.Name == m }) ||
			slices.ContainsFunc(c.Abilities, func(a Ability) bool { return a.Name == m }) {
			n++
		}
	}
	return n
}
