package cardset

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"sync"
)

// DefaultTCGdexBase is the English TCGdex REST API.
const DefaultTCGdexBase = "https://api.tcgdex.net/v2/en"

// TCGdex reads raw set and card data from TCGdex, uncached, for the
// maintainer's import: an import must always see current upstream data.
// It is the only program code that talks to the TCGdex API; the playmat
// reads published card sets (internal/catalog).
type TCGdex struct {
	Base string // default DefaultTCGdexBase
	get  httpGetter
}

// TCGdexSetSummary is one entry of TCGdex's set list.
type TCGdexSetSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CardCount struct {
		Total    int `json:"total"`
		Official int `json:"official"`
	} `json:"cardCount"`
}

// TCGdexSet is a set's details, with its card list.
type TCGdexSet struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ReleaseDate string `json:"releaseDate"`
	Serie       struct {
		ID   string `json:"id"`
		Name string `json:"name"`
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

func (t *TCGdex) base() string {
	if t.Base == "" {
		return DefaultTCGdexBase
	}
	return strings.TrimRight(t.Base, "/")
}

// Sets returns TCGdex's English set list.
func (t *TCGdex) Sets(ctx context.Context) ([]TCGdexSetSummary, error) {
	var out []TCGdexSetSummary
	if err := t.get.getJSON(ctx, t.base()+"/sets", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Set returns a set's details. A set TCGdex does not have is ErrNotFound.
func (t *TCGdex) Set(ctx context.Context, id string) (TCGdexSet, error) {
	var out TCGdexSet
	err := t.get.getJSON(ctx, t.base()+"/sets/"+url.PathEscape(id), &out)
	return out, err
}

// Card returns a print's raw card body. A card TCGdex does not have is
// ErrNotFound.
func (t *TCGdex) Card(ctx context.Context, id string) (json.RawMessage, error) {
	var out json.RawMessage
	if err := t.get.getJSON(ctx, t.base()+"/cards/"+url.PathEscape(id), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Cards fetches the card bodies of ids, workers at a time (8 if workers is
// not positive). It returns the bodies it got and the error for each id it
// could not get.
func (t *TCGdex) Cards(ctx context.Context, ids []string, workers int) (map[string]json.RawMessage, map[string]error) {
	if workers <= 0 {
		workers = 8
	}
	bodies := make(map[string]json.RawMessage, len(ids))
	errs := map[string]error{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, err := t.Card(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs[id] = err
				return
			}
			bodies[id] = b
		}(id)
	}
	wg.Wait()
	return bodies, errs
}
