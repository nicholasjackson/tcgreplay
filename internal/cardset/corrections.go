package cardset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Corrections is a set's corrections file: hand corrections to card fields
// and acknowledgements of report items that need no change. People and the
// card-set import skill write it; the import only reads it.
type Corrections struct {
	Corrections  []Correction `json:"corrections"`
	Acknowledged []Ack        `json:"acknowledged"`
}

// Correction sets one field of one print. Path is an RFC 6901 JSON Pointer
// into the print's JSON form ("/text/en/effect", "/hp", "/images"); Value
// is any JSON value.
type Correction struct {
	Card   string          `json:"card"`
	Path   string          `json:"path"`
	Value  json.RawMessage `json:"value"`
	Reason string          `json:"reason"`
}

// Ack records why a report item needs no change. Field, if set, limits it
// to that field's item (one disagreement among several on a card).
type Ack struct {
	Card   string `json:"card"`
	Kind   string `json:"kind"`
	Field  string `json:"field,omitempty"`
	Reason string `json:"reason"`
}

// ReadCorrections reads a corrections file. A missing file is no
// corrections. Unknown fields, and entries without a card, path, kind or
// reason, are refused.
func ReadCorrections(path string) (*Corrections, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Corrections{}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Corrections
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var errs []error
	for i, x := range c.Corrections {
		switch {
		case x.Card == "" || x.Path == "":
			errs = append(errs, fmt.Errorf("correction %d: needs a card and a path", i))
		case len(x.Value) == 0:
			errs = append(errs, fmt.Errorf("correction %d (%s %s): needs a value", i, x.Card, x.Path))
		case strings.TrimSpace(x.Reason) == "":
			errs = append(errs, fmt.Errorf("correction %d (%s %s): needs a reason", i, x.Card, x.Path))
		}
	}
	for i, a := range c.Acknowledged {
		switch {
		case a.Card == "" || a.Kind == "":
			errs = append(errs, fmt.Errorf("acknowledgement %d: needs a card and a kind", i))
		case strings.TrimSpace(a.Reason) == "":
			errs = append(errs, fmt.Errorf("acknowledgement %d (%s %s): needs a reason", i, a.Card, a.Kind))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// applied is a correction that was applied, with the value the field held
// before it (the merged sources' value; nil if the field was absent).
type applied struct {
	Correction
	Before    any
	Redundant bool // the sources already give the corrected value
}

// applyCorrections applies cs to the set's prints in order. A correction
// naming a card the set does not hold, or a path the format does not
// define, is an error naming it.
func applyCorrections(s *Set, cs []Correction) ([]applied, error) {
	var out []applied
	var errs []error
	for _, c := range cs {
		p, ok := s.Print(c.Card)
		if !ok {
			errs = append(errs, fmt.Errorf("correction to card %s: set %s has no such card", c.Card, s.Set.ID))
			continue
		}
		np, a, err := applyCorrection(*p, c)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		*p = np
		out = append(out, a)
	}
	return out, errors.Join(errs...)
}

// applyCorrection sets one field of a print through its JSON form, then
// decodes the result strictly, so a path the format does not define fails.
func applyCorrection(p Print, c Correction) (Print, applied, error) {
	fail := func(err error) (Print, applied, error) {
		return Print{}, applied{}, fmt.Errorf("card %s: path %s: %w", c.Card, c.Path, err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		return fail(err)
	}
	var doc any
	if err := decodeNumbers(b, &doc); err != nil {
		return fail(err)
	}
	var value any
	if err := decodeNumbers(c.Value, &value); err != nil {
		return fail(fmt.Errorf("value: %w", err))
	}
	tokens, err := parsePointer(c.Path)
	if err != nil {
		return fail(err)
	}
	if len(tokens) == 0 {
		return fail(errors.New("a correction cannot replace the whole card"))
	}
	doc, before, err := setPointer(doc, tokens, value)
	if err != nil {
		return fail(err)
	}
	nb, err := json.Marshal(doc)
	if err != nil {
		return fail(err)
	}
	var np Print
	dec := json.NewDecoder(bytes.NewReader(nb))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&np); err != nil {
		return fail(fmt.Errorf("not a field of the card format: %w", err))
	}
	if np.ID != p.ID {
		return fail(errors.New("a correction cannot change the card's id"))
	}
	return np, applied{Correction: c, Before: before, Redundant: sameValue(before, value)}, nil
}

func decodeNumbers(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	return dec.Decode(v)
}

// parsePointer splits an RFC 6901 JSON Pointer into unescaped tokens.
func parsePointer(ptr string) ([]string, error) {
	if ptr == "" {
		return nil, nil
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, errors.New(`a JSON Pointer starts with "/"`)
	}
	parts := strings.Split(ptr[1:], "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(p, "~1", "/"), "~0", "~")
	}
	return parts, nil
}

// setPointer sets the value at tokens inside doc, returning the new doc and
// the value it replaced (nil if none). Missing object members on the way
// are created; array elements must exist, except that the last token may
// be "-" or the array's length to append.
func setPointer(doc any, tokens []string, value any) (any, any, error) {
	tok := tokens[0]
	last := len(tokens) == 1
	switch node := doc.(type) {
	case map[string]any:
		if last {
			before := node[tok]
			node[tok] = value
			return node, before, nil
		}
		child, ok := node[tok]
		if !ok || child == nil {
			child = map[string]any{}
		}
		nc, before, err := setPointer(child, tokens[1:], value)
		if err != nil {
			return nil, nil, err
		}
		node[tok] = nc
		return node, before, nil
	case []any:
		i := len(node)
		if tok != "-" {
			n, err := strconv.Atoi(tok)
			if err != nil || n < 0 || n > len(node) {
				return nil, nil, fmt.Errorf("no element %q in a list of %d", tok, len(node))
			}
			i = n
		}
		if i == len(node) {
			if !last {
				return nil, nil, fmt.Errorf("no element %q in a list of %d", tok, len(node))
			}
			return append(node, value), nil, nil
		}
		if last {
			before := node[i]
			node[i] = value
			return node, before, nil
		}
		nc, before, err := setPointer(node[i], tokens[1:], value)
		if err != nil {
			return nil, nil, err
		}
		node[i] = nc
		return node, before, nil
	case nil:
		return setPointer(map[string]any{}, tokens, value)
	default:
		return nil, nil, fmt.Errorf("%q is inside a value that is not an object or list", tok)
	}
}

// sameValue reports whether a correction's value is what the field already
// held: equal JSON, or text equal but for cosmetic differences.
func sameValue(before, value any) bool {
	if before == nil {
		return false
	}
	if bs, ok := before.(string); ok {
		if vs, ok := value.(string); ok {
			return compareKey(bs) == compareKey(vs)
		}
	}
	return reflect.DeepEqual(before, value)
}
