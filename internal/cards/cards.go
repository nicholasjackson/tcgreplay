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
}

// Client is a TCGdex client with a disk cache.
type Client struct {
	Base     string // default https://api.tcgdex.net/v2/en
	CacheDir string
	HTTP     *http.Client
	// Offline disables network access; only cached data is used.
	Offline bool

	mu       sync.Mutex
	mem      map[string]*Card
	setIndex map[string]string // upper set code -> set id
	indexed  bool
}

// NewClient returns a client caching under dir.
func NewClient(dir string) *Client {
	return &Client{
		Base:     "https://api.tcgdex.net/v2/en",
		CacheDir: dir,
		HTTP:     &http.Client{Timeout: 20 * time.Second},
		mem:      map[string]*Card{},
	}
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

// getJSON fetches path relative to Base, caching the raw body at cacheFile.
func (c *Client) getJSON(ctx context.Context, path, cacheFile string, out any) error {
	if b, err := os.ReadFile(cacheFile); err == nil {
		return json.Unmarshal(b, out)
	}
	if c.Offline {
		return fmt.Errorf("%s: not cached and offline: %w", path, ErrNotFound)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("tcgdex %s: %w", path, ErrNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tcgdex %s: %s", path, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("tcgdex %s: %w", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(cacheFile), 0o755); err == nil {
		tmp := cacheFile + ".tmp"
		if os.WriteFile(tmp, b, 0o644) == nil {
			os.Rename(tmp, cacheFile)
		}
	}
	return nil
}

// Card implements DB.
func (c *Client) Card(ctx context.Context, id string) (*Card, error) {
	if id == "" {
		return nil, fmt.Errorf("empty card id: %w", ErrNotFound)
	}
	c.mu.Lock()
	if card, ok := c.mem[id]; ok {
		c.mu.Unlock()
		return card, nil
	}
	c.mu.Unlock()
	var card Card
	err := c.getJSON(ctx, "/cards/"+url.PathEscape(id), filepath.Join(c.CacheDir, "cards", safeName(id)+".json"), &card)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.mem[id] = &card
	c.mu.Unlock()
	return &card, nil
}

// Cached implements DB.
func (c *Client) Cached(id string) *Card {
	c.mu.Lock()
	card, ok := c.mem[id]
	c.mu.Unlock()
	if ok {
		return card
	}
	b, err := os.ReadFile(filepath.Join(c.CacheDir, "cards", safeName(id)+".json"))
	if err != nil {
		return nil
	}
	var cc Card
	if json.Unmarshal(b, &cc) != nil {
		return nil
	}
	c.mu.Lock()
	c.mem[id] = &cc
	c.mu.Unlock()
	return &cc
}

type setSummary struct {
	ID string `json:"id"`
}

type setDetail struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Abbreviation struct {
		Official string `json:"official"`
	} `json:"abbreviation"`
	TCGOnline string `json:"tcgOnline"`
	Cards     []struct {
		ID      string `json:"id"`
		LocalID string `json:"localId"`
		Name    string `json:"name"`
	} `json:"cards"`
}

// codeAliases maps TCG Live set codes that differ from TCGdex
// abbreviations.
var codeAliases = map[string]string{
	"PR-SV": "SVP",
	"PR-SW": "SWSHP",
}

func (c *Client) set(ctx context.Context, id string) (*setDetail, error) {
	var s setDetail
	err := c.getJSON(ctx, "/sets/"+url.PathEscape(id), filepath.Join(c.CacheDir, "sets", safeName(id)+".json"), &s)
	return &s, err
}

// buildIndex maps set codes to TCGdex set ids. It fetches every set once and
// caches the result.
func (c *Client) buildIndex(ctx context.Context, refresh bool) error {
	indexFile := filepath.Join(c.CacheDir, "set-index.json")
	if !refresh {
		if b, err := os.ReadFile(indexFile); err == nil {
			idx := map[string]string{}
			if json.Unmarshal(b, &idx) == nil {
				c.setIndex = idx
				return nil
			}
		}
	}
	if c.Offline {
		return fmt.Errorf("set index not cached and offline: %w", ErrNotFound)
	}
	var sets []setSummary
	// Always refetch the set list when rebuilding.
	os.Remove(filepath.Join(c.CacheDir, "sets.json"))
	if err := c.getJSON(ctx, "/sets", filepath.Join(c.CacheDir, "sets.json"), &sets); err != nil {
		return err
	}
	idx := map[string]string{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	var firstErr error
	for _, s := range sets {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			d, err := c.set(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil && !errors.Is(err, ErrNotFound) {
					firstErr = err
				}
				return
			}
			for _, code := range []string{d.Abbreviation.Official, d.TCGOnline} {
				if code != "" {
					idx[strings.ToUpper(code)] = d.ID
				}
			}
		}(s.ID)
	}
	wg.Wait()
	if len(idx) == 0 && firstErr != nil {
		return firstErr
	}
	c.setIndex = idx
	if b, err := json.Marshal(idx); err == nil {
		os.MkdirAll(c.CacheDir, 0o755)
		os.WriteFile(indexFile, b, 0o644)
	}
	return nil
}

// Resolve implements DB.
func (c *Client) Resolve(ctx context.Context, setCode, number string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(setCode))
	if a, ok := codeAliases[code]; ok {
		code = a
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setIndex == nil {
		if err := c.buildIndex(ctx, false); err != nil {
			return "", err
		}
	}
	setID, ok := c.setIndex[code]
	if !ok && !c.indexed && !c.Offline {
		// The cached index may predate a new set.
		c.indexed = true
		if err := c.buildIndex(ctx, true); err != nil {
			return "", err
		}
		setID, ok = c.setIndex[code]
	}
	if !ok {
		return "", fmt.Errorf("unknown set code %q: %w", setCode, ErrNotFound)
	}
	s, err := c.set(ctx, setID)
	if err != nil {
		return "", err
	}
	want, numErr := strconv.Atoi(number)
	for _, card := range s.Cards {
		if strings.EqualFold(card.LocalID, number) {
			return card.ID, nil
		}
		if numErr == nil {
			if n, err := strconv.Atoi(card.LocalID); err == nil && n == want {
				return card.ID, nil
			}
		}
	}
	return "", fmt.Errorf("no card %s %s in set %s: %w", setCode, number, setID, ErrNotFound)
}
