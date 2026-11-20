package cardset

import (
	"context"
	"strings"
)

// DefaultMalieBase is the malie.io TCG Live export.
const DefaultMalieBase = "https://cdn.malie.io/file/malie-io/tcgl/export"

// malieLocale is the export's English locale.
const malieLocale = "en-US"

// Malie reads the malie.io TCG Live export, uncached. It is a fan project
// covering only TCG Live's sets (Scarlet & Violet onwards) and is used at
// build time only, to fill gaps in TCGdex's data.
type Malie struct {
	Base string // default DefaultMalieBase
	get  httpGetter
}

// MalieSetRef is one set in the export's index.
type MalieSetRef struct {
	Path string `json:"path"` // set file, relative to Base
	Name string `json:"name"` // may hold HTML (<i>…</i>)
	Num  int    `json:"num"`  // entries in the set file
	Hash string `json:"hash"`
	Abbr string `json:"abbr"` // TCG Live decklist set code, "DLR"
}

// MalieCard is one entry of a set file. A print has two entries: the
// standard one and a reverse-holo one with the same text.
type MalieCard struct {
	Name           string        `json:"name"`
	CardType       string        `json:"card_type"` // POKEMON, TRAINER, ENERGY
	Subtype        string        `json:"subtype"`   // SUPPORTER, ITEM, STADIUM, TOOL, SPECIAL
	Stage          string        `json:"stage"`     // BASIC, STAGE1, STAGE2, ...
	StageText      string        `json:"stage_text"`
	HP             flexInt       `json:"hp"`
	Types          []string      `json:"types"`
	Weakness       *MalieTypeMod `json:"weakness"`
	Resistance     *MalieTypeMod `json:"resistance"`
	Retreat        flexInt       `json:"retreat"`
	RegulationMark string        `json:"regulation_mark"`
	Tags           []string      `json:"tags"` // EX_LOWER, TERA, ...
	Text           []MalieText   `json:"text"`
	Number         struct {
		Full    string  `json:"full"`
		Numeric flexInt `json:"numeric"`
	} `json:"collector_number"`
	Ext struct {
		TCGL struct {
			CardID     string `json:"cardID"`     // TCG Live log code, "me6_1"
			LongFormID string `json:"longFormID"` // holds _std_ for the standard entry
			Key        string `json:"key"`
		} `json:"tcgl"`
	} `json:"ext"`
	Images struct {
		TCGL struct {
			JPG struct {
				Front string `json:"front"`
			} `json:"jpg"`
		} `json:"tcgl"`
	} `json:"images"`
}

// MalieTypeMod is a weakness or resistance.
type MalieTypeMod struct {
	Types    []string `json:"types"`
	Operator string   `json:"operator"`
	Amount   flexInt  `json:"amount"`
}

// MalieText is one block of a card's text.
type MalieText struct {
	Kind   string       `json:"kind"` // ATTACK, ABILITY, TEXT_BOX, RULE_BOX, REMINDER
	Name   string       `json:"name"`
	Text   string       `json:"text"`
	Cost   []string     `json:"cost"`
	Damage *MalieDamage `json:"damage"`
}

// MalieDamage is an attack's damage: an amount and a suffix (×, +, -).
type MalieDamage struct {
	Amount flexInt `json:"amount"`
	Suffix string  `json:"suffix"`
}

func (m *Malie) base() string {
	if m.Base == "" {
		return DefaultMalieBase
	}
	return strings.TrimRight(m.Base, "/")
}

// Index returns the export's English sets by malie set key ("me6").
func (m *Malie) Index(ctx context.Context) (map[string]MalieSetRef, error) {
	var idx map[string]map[string]MalieSetRef
	if err := m.get.getJSON(ctx, m.base()+"/index.json", &idx); err != nil {
		return nil, err
	}
	en := idx[malieLocale]
	if en == nil {
		en = map[string]MalieSetRef{}
	}
	return en, nil
}

// SetFile returns a set file's entries; path is the index's Path.
func (m *Malie) SetFile(ctx context.Context, path string) ([]MalieCard, error) {
	var out []MalieCard
	if err := m.get.getJSON(ctx, m.base()+"/"+strings.TrimLeft(path, "/"), &out); err != nil {
		return nil, err
	}
	return out, nil
}
