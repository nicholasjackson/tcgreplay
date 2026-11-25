// Package importreports keeps battle logs that failed to import, so the
// maintainer can see what the importer could not read. A report is saved
// only when the player agrees (the assistant asks first, through the
// report_import tool), with the players' names replaced, in a private
// prefix of the shared store, and it is deleted after Keep (14 days by
// default). Only admins can read reports, from the admin page.
package importreports

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nicholasjackson/tcgreplay/internal/importer"
	"github.com/nicholasjackson/tcgreplay/internal/spaces"
)

// Prefix is where reports live in the shared store: one object per report
// at <Prefix><YYYY-MM-DD>/<id>.json, the UTC day it was saved. Objects are
// private; the day in the key is what the sweep deletes by.
const Prefix = "import-reports/"

// DefaultKeep is how long a report is kept: the same 14 days as the error
// records it explains.
const DefaultKeep = 14 * 24 * time.Hour

// sweepEvery is how often old reports are deleted, after a first sweep a
// minute after start-up.
const (
	sweepFirst = time.Minute
	sweepEvery = time.Hour
)

// Store is the private object store reports are kept in (a
// *spaces.Client).
type Store interface {
	Put(ctx context.Context, key string, body []byte, o spaces.PutOptions) error
	Get(ctx context.Context, key string) ([]byte, error)
	List(ctx context.Context, prefix, token string) (spaces.ListPage, error)
	Delete(ctx context.Context, keys []string) error
}

// Report is one saved import failure, as stored.
type Report struct {
	ID        string    `json:"id"`
	Saved     time.Time `json:"saved"`
	Player    string    `json:"player,omitempty"`    // the player's pseudonymous ID, as the records name them
	Assistant string    `json:"assistant,omitempty"` // the assistant that sent it
	// Error is the import's error, as the assistant saw it, names
	// replaced.
	Error   string `json:"error,omitempty"`
	Log     string `json:"log"`
	Deck    string `json:"deck,omitempty"`
	OppDeck string `json:"opp_deck,omitempty"`
	// Names is how many player names were found and replaced (0 when the
	// log names none the playmat recognises).
	Names int `json:"names"`
}

// Summary is a report as the admin page lists it.
type Summary struct {
	ID    string
	Saved time.Time
	Error string
}

// Service saves, lists, reads and expires reports.
type Service struct {
	Store Store
	// Now is the clock; nil means time.Now.
	Now func() time.Time
	// Keep is how long a report is kept; zero means DefaultKeep.
	Keep time.Duration
	// Logf logs sweeps; nil means log.Printf.
	Logf func(format string, args ...any)
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) keep() time.Duration {
	if s.Keep > 0 {
		return s.Keep
	}
	return DefaultKeep
}

func (s *Service) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
		return
	}
	log.Printf(format, args...)
}

// ErrNotFound means no report has the ID asked for (or it has expired).
var ErrNotFound = errors.New("no import report with that ID")

// idRe is a report ID: ir-<YYYYMMDD>-<8 hex>.
var idRe = regexp.MustCompile(`^ir-(\d{4})(\d{2})(\d{2})-[0-9a-f]{8}$`)

// key is the store key of the report id, or "" for a malformed ID.
func key(id string) string {
	m := idRe.FindStringSubmatch(id)
	if m == nil {
		return ""
	}
	return fmt.Sprintf("%s%s-%s-%s/%s.json", Prefix, m[1], m[2], m[3], id)
}

func newID(t time.Time) string {
	b := make([]byte, 4)
	rand.Read(b)
	return "ir-" + t.Format("20060102") + "-" + hex.EncodeToString(b)
}

// Save stores r with the players' names replaced by "Player 1",
// "Player 2" and so on (me, the log's owner, first when given) in its log
// and error, and returns it as stored, with its ID.
func (s *Service) Save(ctx context.Context, r Report, me string) (Report, error) {
	if strings.TrimSpace(r.Log) == "" {
		return Report{}, errors.New("there is no battle log to report")
	}
	now := s.now()
	names := importer.Names(r.Log, me)
	r.Log = Hide(r.Log, names)
	r.Error = Hide(r.Error, names)
	r.Names = len(names)
	r.ID, r.Saved = newID(now), now
	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Report{}, err
	}
	if err := s.Store.Put(ctx, key(r.ID), body, spaces.PutOptions{ContentType: "application/json"}); err != nil {
		return Report{}, fmt.Errorf("the report could not be saved: %w", err)
	}
	return r, nil
}

// Get returns the stored report id as JSON.
func (s *Service) Get(ctx context.Context, id string) ([]byte, error) {
	k := key(id)
	if k == "" {
		return nil, ErrNotFound
	}
	b, err := s.Store.Get(ctx, k)
	if errors.Is(err, spaces.ErrNotFound) {
		return nil, ErrNotFound
	}
	return b, err
}

// keys lists every report key, oldest first.
func (s *Service) keys(ctx context.Context) ([]string, error) {
	var out []string
	token := ""
	for {
		page, err := s.Store.List(ctx, Prefix, token)
		if err != nil {
			return nil, err
		}
		for _, k := range page.Keys {
			if strings.HasSuffix(k, ".json") {
				out = append(out, k)
			}
		}
		if page.Next == "" {
			break
		}
		token = page.Next
	}
	sort.Strings(out)
	return out, nil
}

// idOf is the report ID in a store key.
func idOf(k string) string {
	return strings.TrimSuffix(k[strings.LastIndexByte(k, '/')+1:], ".json")
}

// List returns up to limit reports, newest first, with the first line of
// each one's error.
func (s *Service) List(ctx context.Context, limit int) ([]Summary, error) {
	keys, err := s.keys(ctx)
	if err != nil {
		return nil, err
	}
	slices.Reverse(keys)
	if limit > 0 && len(keys) > limit {
		keys = keys[:limit]
	}
	out := make([]Summary, 0, len(keys))
	for _, k := range keys {
		sum := Summary{ID: idOf(k)}
		if b, err := s.Store.Get(ctx, k); err == nil {
			var r Report
			if json.Unmarshal(b, &r) == nil {
				sum.Saved = r.Saved
				sum.Error, _, _ = strings.Cut(r.Error, "\n")
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

// Sweep deletes the reports saved on a UTC day that ended longer ago than
// Keep, and returns how many it deleted.
func (s *Service) Sweep(ctx context.Context) (int, error) {
	keys, err := s.keys(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := s.now().Add(-s.keep())
	var old []string
	for _, k := range keys {
		day, err := time.Parse("2006-01-02", strings.SplitN(strings.TrimPrefix(k, Prefix), "/", 2)[0])
		if err != nil {
			continue
		}
		if day.Add(24 * time.Hour).Before(cutoff) {
			old = append(old, k)
		}
	}
	if len(old) == 0 {
		return 0, nil
	}
	if err := s.Store.Delete(ctx, old); err != nil {
		return 0, err
	}
	return len(old), nil
}

// Run sweeps a minute after start-up and then every hour, until ctx is
// done.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTimer(sweepFirst)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n, err := s.Sweep(ctx)
			switch {
			case err != nil:
				s.logf("warning: import reports: sweep failed: %v", err)
			case n > 0:
				s.logf("import reports: deleted %d older than %s", n, s.keep())
			}
			t.Reset(sweepEvery)
		}
	}
}

// Hide replaces each of names in text with "Player N", N its place in
// names, wherever it stands as a whole word (a possessive "'s" still
// matches). Longer names are replaced first, so a name inside another is
// never split.
func Hide(text string, names []string) string {
	if text == "" || len(names) == 0 {
		return text
	}
	order := make([]int, len(names))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return len(names[order[a]]) > len(names[order[b]]) })
	// Placeholders first, so a later name cannot match inside "Player 1".
	marks := make([]string, len(names))
	for _, i := range order {
		marks[i] = fmt.Sprintf("\x00%d\x00", i)
		text = replaceWord(text, names[i], marks[i])
	}
	for i := range names {
		text = strings.ReplaceAll(text, marks[i], fmt.Sprintf("Player %d", i+1))
	}
	return text
}

// replaceWord replaces every whole-word occurrence of word in text.
func replaceWord(text, word, with string) string {
	if word == "" {
		return text
	}
	var b strings.Builder
	for {
		i := strings.Index(text, word)
		if i < 0 {
			b.WriteString(text)
			return b.String()
		}
		end := i + len(word)
		before, _ := utf8.DecodeLastRuneInString(text[:i])
		after, _ := utf8.DecodeRuneInString(text[end:])
		if (i == 0 || !wordRune(before)) && (end == len(text) || !wordRune(after)) {
			b.WriteString(text[:i])
			b.WriteString(with)
		} else {
			b.WriteString(text[:end])
		}
		text = text[end:]
	}
}

func wordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
