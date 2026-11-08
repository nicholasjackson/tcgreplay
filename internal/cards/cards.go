// Package cards holds the playmat's runtime card definitions, the card
// lookup interface, TCG Live log-code and name matching, and the card image
// cache. Card definitions themselves come from the published card sets
// (internal/catalog).
package cards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotFound means there is no such card or set.
var ErrNotFound = errors.New("not found")

// ErrUnknownSet means a name given to name a set names no set.
var ErrUnknownSet = fmt.Errorf("no such set: %w", ErrNotFound)

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
	ID       string `json:"id"`
	LocalID  string `json:"localId"`
	Name     string `json:"name"`
	Category string `json:"category"` // Pokemon, Trainer, Energy
	// Image is the first of Images (empty when the card has no art), for
	// readers that only ask whether a card has art.
	Image string `json:"image,omitempty"`
	// Images is the print's whole ordered list of image sources, in the
	// order the board tries them; empty means no art. Cards that set only
	// Image (test pools) are read through ImageList.
	Images         []ImageRef  `json:"images,omitempty"`
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

// ImageRef is one source of a card's image.
type ImageRef struct {
	URL string `json:"url"`
	// File marks a complete image file used for every size (a malie.io
	// jpg, say). Otherwise URL is a TCGdex base path, to which the board
	// appends /low.webp or /high.webp.
	File bool `json:"file,omitempty"`
}

// ImageList is the card's ordered image sources: Images, or one entry
// made from Image when only Image is set, or nil for a card with no art.
func (c *Card) ImageList() []ImageRef {
	if len(c.Images) > 0 {
		return c.Images
	}
	if c.Image != "" {
		return []ImageRef{{URL: c.Image, File: ImageIsFile(c.Image)}}
	}
	return nil
}

// ImageIsFile reports whether an image address names a complete image
// file (its last path segment ends in .jpg, .jpeg, .png or .webp) rather
// than a TCGdex base path, which ends in the card number.
func ImageIsFile(addr string) bool {
	if u, err := url.Parse(addr); err == nil {
		addr = u.Path
	}
	ext := strings.ToLower(path.Ext(path.Base(addr)))
	return ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp"
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

// basicEnergyArt is Crown Zenith's Basic Energy art. TCGdex has no images
// for the Scarlet & Violet and Mega Evolution Energy prints that decks
// use, and every other print with art is a gold hyper rare.
var basicEnergyArt = map[string]string{
	"Grass": "152", "Fire": "153", "Water": "154", "Lightning": "155",
	"Psychic": "156", "Fighting": "157", "Darkness": "158", "Metal": "159",
}

// EnergyArt gives a Basic Energy card without an image the art of the same
// type (Crown Zenith's), as the playmat has always shown it. Built card
// sets apply it when a print is converted (cardset.Print.Card); the art is
// never stored in the built set.
func EnergyArt(c *Card) { energyArt(c) }

// energyArt gives a Basic Energy card without an image the art of the
// same type. Only the picture is borrowed; the card keeps its identity.
func energyArt(c *Card) {
	if c.Image != "" || c.Category != "Energy" || c.EnergyType != "Normal" {
		return
	}
	for t, n := range basicEnergyArt {
		if strings.Contains(c.Name, t) {
			c.Image = "https://assets.tcgdex.net/en/swsh/swsh12.5/" + n
			c.Images = []ImageRef{{URL: c.Image}}
			return
		}
	}
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

// NameKey normalises a card name for lookup: case and apostrophe style
// differ between TCG Live and TCGdex.
func NameKey(name string) string { return nameKey(name) }

func nameKey(name string) string {
	name = strings.NewReplacer("’", "'", "‘", "'").Replace(name)
	return strings.ToLower(strings.Join(strings.Fields(name), " "))
}

// SameName reports whether two card names match, ignoring case, spacing
// and apostrophe style.
func SameName(a, b string) bool { return nameKey(a) == nameKey(b) }
