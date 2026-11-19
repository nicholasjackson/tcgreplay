// Package plugin builds the ChatGPT plugin package that is uploaded to
// OpenAI's plugin dashboard: a ZIP holding plugin.json, mcp.json and the
// listing icon.
//
// Only the hand-written parts of the listing live in plugin/listing.json.
// The public address comes from TCGREPLAY_PUBLIC_URL in .do/app.yaml, and
// the fan-project notice, card-data credit and support link from
// internal/notice/notice.json, so the listing cannot drift from the
// website. Build refuses, naming the rule, anything OpenAI's form would
// reject (developers.openai.com/plugins, read 2026-11-12).
package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/png" // registers the PNG decoder for image.DecodeConfig
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nicholasjackson/tcgreplay/internal/appspec"
)

// Sources are the contents of the files a package is built from.
type Sources struct {
	// AppSpec is .do/app.yaml.
	AppSpec string
	// Notice is internal/notice/notice.json; empty means the file is missing.
	Notice string
	// Listing is plugin/listing.json.
	Listing string
	// ReleaseNotes is plugin/release-notes.md.
	ReleaseNotes string
	// Icon is plugin/assets/icon.png.
	Icon []byte
}

// OpenAI's limits on a plugin package.
const (
	maxNameChars        = 30
	maxDescriptionChars = 4000
	maxPrompts          = 3
	maxPromptChars      = 128
	positiveCases       = 5
	negativeCases       = 3
	maxURLChars         = 1024
	minIconPx           = 48
	maxIconPx           = 4096
	maxIconBytes        = 5 << 20
)

// Paths inside the package.
const (
	iconPath   = "assets/icon.png"
	pluginPath = "plugin.json"
	mcpPath    = "mcp.json"
)

// publicPlaceholder is what the listing source writes where it needs the
// playmat's address; Build replaces it with the App Platform address.
const publicPlaceholder = "{public}"

// demoRecordingPath is the one listing field that may hold a literal URL:
// the externally hosted demo video.
const demoRecordingPath = "review.demo_recording_url"

var (
	nameRule        = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	versionRule     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	colourRule      = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	placeholderRule = regexp.MustCompile(`\{[A-Za-z_]+\}`)
	// forbiddenKeys may appear nowhere in the listing source: reviewer
	// credentials go only in the dashboard, and apps and hooks are blocked
	// in submitted plugins.
	forbiddenKeys = []string{"clientSecret", "client_secret", "test_credentials", "testCredentials", "reviewer_instructions", "password", "apps", "hooks"}
)

// fixedTime is every ZIP entry's modification time, so two builds from
// the same sources are byte-identical.
var fixedTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// notice is internal/notice/notice.json.
type notice struct {
	Notice string `json:"notice"`
	Credit struct {
		Lead    string `json:"lead"`
		Sources []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"sources"`
	} `json:"credit"`
	Help struct {
		Label string `json:"label"`
		URL   string `json:"url"`
	} `json:"help"`
}

// creditText renders the credit as one sentence: "Card data and images
// from TCGdex (https://tcgdex.dev) and malie.io (https://malie.io)."
func (n notice) creditText() string {
	parts := make([]string, len(n.Credit.Sources))
	for i, s := range n.Credit.Sources {
		parts[i] = fmt.Sprintf("%s (%s)", s.Name, s.URL)
	}
	list := strings.Join(parts, ", ")
	if len(parts) > 1 {
		list = strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
	}
	return n.Credit.Lead + " " + list + "."
}

// listing is plugin/listing.json, the hand-written part of the listing.
type listing struct {
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	DisplayName      string   `json:"displayName"`
	ShortDescription string   `json:"shortDescription"`
	Description      string   `json:"description"`
	DeveloperName    string   `json:"developerName"`
	Category         string   `json:"category"`
	Capabilities     []string `json:"capabilities"`
	DefaultPrompt    []string `json:"defaultPrompt"`
	BrandColor       string   `json:"brandColor"`
	Review           review   `json:"review"`
}

type review struct {
	TestCases        testCases `json:"test_cases"`
	DemoRecordingURL string    `json:"demo_recording_url,omitempty"`
}

type testCases struct {
	Positive []positiveCase `json:"positive"`
	Negative []negativeCase `json:"negative"`
}

type positiveCase struct {
	Description      string `json:"description"`
	Prompt           string `json:"prompt"`
	ToolsTriggered   string `json:"tools_triggered"`
	ExpectedBehavior string `json:"expected_behavior"`
}

type negativeCase struct {
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

// pluginJSON is plugin.json (agent-plugins schema 1.0.0).
type pluginJSON struct {
	Schema      string           `json:"$schema"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Homepage    string           `json:"homepage"`
	Repository  string           `json:"repository"`
	Extensions  pluginExtensions `json:"extensions"`
}

type pluginExtensions struct {
	OpenAI openAIExtension `json:"com.openai"`
}

type openAIExtension struct {
	Interface   iface       `json:"interface"`
	Review      review      `json:"review"`
	Publication publication `json:"publication"`
}

type iface struct {
	DisplayName       string   `json:"displayName"`
	ShortDescription  string   `json:"shortDescription"`
	LongDescription   string   `json:"longDescription"`
	DeveloperName     string   `json:"developerName"`
	Category          string   `json:"category"`
	Capabilities      []string `json:"capabilities"`
	WebsiteURL        string   `json:"websiteURL"`
	PrivacyPolicyURL  string   `json:"privacyPolicyURL"`
	TermsOfServiceURL string   `json:"termsOfServiceURL"`
	SupportURL        string   `json:"supportURL"`
	DefaultPrompt     []string `json:"defaultPrompt"`
	BrandColor        string   `json:"brandColor,omitempty"`
	ComposerIcon      string   `json:"composerIcon"`
	Logo              string   `json:"logo"`
}

type publication struct {
	ReleaseNotes string `json:"release_notes"`
}

// mcpJSON is mcp.json: the one MCP server, the hosted playmat, signed in
// to through OAuth with dynamic client registration.
type mcpJSON struct {
	Schema     string               `json:"$schema"`
	MCPServers map[string]mcpServer `json:"mcpServers"`
}

type mcpServer struct {
	Type       string        `json:"type"`
	URL        string        `json:"url"`
	Extensions mcpExtensions `json:"extensions"`
}

type mcpExtensions struct {
	OpenAI struct {
		Auth struct {
			Type   string `json:"type"`
			Client struct {
				Mode string `json:"mode"`
			} `json:"client"`
		} `json:"auth"`
	} `json:"com.openai"`
}

// Build checks the sources against OpenAI's limits and returns the plugin
// package as ZIP bytes. The error names the first rule broken.
func Build(src Sources) ([]byte, error) {
	public, err := appspec.PublicURL(src.AppSpec)
	if err != nil {
		return nil, err
	}
	n, err := parseNotice(src.Notice)
	if err != nil {
		return nil, err
	}
	l, err := parseListing(src.Listing, n, public.String())
	if err != nil {
		return nil, err
	}
	if err := checkIcon(src.Icon); err != nil {
		return nil, err
	}
	notes := strings.TrimSpace(src.ReleaseNotes)
	if notes == "" {
		return nil, errors.New("release notes: plugin/release-notes.md is missing or empty")
	}

	p, m, err := compose(l, n, public, notes)
	if err != nil {
		return nil, err
	}
	pj, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	mj, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return writeZip([]zipEntry{
		{pluginPath, append(pj, '\n')},
		{mcpPath, append(mj, '\n')},
		{iconPath, src.Icon},
	})
}

// parseNotice reads the shared notice file.
func parseNotice(text string) (notice, error) {
	var n notice
	if strings.TrimSpace(text) == "" {
		return n, errors.New("notice: internal/notice/notice.json is missing or empty")
	}
	if err := json.Unmarshal([]byte(text), &n); err != nil {
		return n, fmt.Errorf("notice: internal/notice/notice.json: %v", err)
	}
	switch {
	case n.Notice == "":
		return n, errors.New("notice: internal/notice/notice.json has no notice")
	case n.Credit.Lead == "" || len(n.Credit.Sources) == 0:
		return n, errors.New("notice: internal/notice/notice.json has no card-data credit")
	case n.Help.URL == "":
		return n, errors.New("notice: internal/notice/notice.json has no help (support) link")
	}
	return n, nil
}

// parseListing reads the listing source: it refuses forbidden keys, and
// any address, notice or support link spelled out rather than left to the
// builder; it fills in the {public} placeholder, then decodes strictly.
func parseListing(text string, n notice, public string) (listing, error) {
	var l listing
	var raw any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return l, fmt.Errorf("listing: plugin/listing.json: %v", err)
	}
	if err := checkSource(raw, "", n); err != nil {
		return l, err
	}
	raw = fill(raw, public)
	filled, err := json.Marshal(raw)
	if err != nil {
		return l, err
	}
	dec := json.NewDecoder(bytes.NewReader(filled))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return l, fmt.Errorf("listing: plugin/listing.json: %v", err)
	}
	return l, nil
}

// checkSource walks the listing source refusing forbidden keys, literal
// addresses (outside the demo video's), the notice text and the support
// link, and placeholders other than {public}.
func checkSource(v any, path string, n notice) error {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			for _, bad := range forbiddenKeys {
				if strings.EqualFold(k, bad) {
					return fmt.Errorf("listing: %q is not allowed in the listing source (reviewer credentials go only in OpenAI's dashboard; apps and hooks are blocked)", join(path, k))
				}
			}
			if err := checkSource(v[k], join(path, k), n); err != nil {
				return err
			}
		}
	case []any:
		for i, e := range v {
			if err := checkSource(e, fmt.Sprintf("%s[%d]", path, i), n); err != nil {
				return err
			}
		}
	case string:
		if path != demoRecordingPath && (strings.Contains(v, "http://") || strings.Contains(v, "https://")) {
			return fmt.Errorf("listing: %s spells out an address; write %s for the playmat's address, the builder adds the website, privacy, terms and support links", path, publicPlaceholder)
		}
		if strings.Contains(v, n.Notice) {
			return fmt.Errorf("listing: %s repeats the fan-project notice; the builder appends it from internal/notice/notice.json", path)
		}
		if strings.Contains(v, strings.TrimPrefix(strings.TrimPrefix(n.Help.URL, "https://"), "http://")) {
			return fmt.Errorf("listing: %s spells out the support link; the builder takes it from internal/notice/notice.json", path)
		}
		for _, p := range placeholderRule.FindAllString(v, -1) {
			if p != publicPlaceholder {
				return fmt.Errorf("listing: %s has an unknown placeholder %s (only %s is filled in)", path, p, publicPlaceholder)
			}
		}
	}
	return nil
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// fill replaces {public} in every string of v with the public address.
func fill(v any, public string) any {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			v[k] = fill(e, public)
		}
	case []any:
		for i, e := range v {
			v[i] = fill(e, public)
		}
	case string:
		return strings.ReplaceAll(v, publicPlaceholder, public)
	}
	return v
}

// checkIcon refuses an icon that is not a square PNG of 48 to 4096 px
// under 5 MiB.
func checkIcon(b []byte) error {
	if len(b) == 0 {
		return errors.New("icon: plugin/assets/icon.png is missing or empty")
	}
	if len(b) > maxIconBytes {
		return fmt.Errorf("icon: %d bytes, over OpenAI's 5 MiB limit", len(b))
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || format != "png" {
		return errors.New("icon: not a PNG image")
	}
	if cfg.Width != cfg.Height {
		return fmt.Errorf("icon: %dx%d px, must be square", cfg.Width, cfg.Height)
	}
	if cfg.Width < minIconPx || cfg.Width > maxIconPx {
		return fmt.Errorf("icon: %d px across, must be %d to %d px", cfg.Width, minIconPx, maxIconPx)
	}
	return nil
}

// compose builds plugin.json and mcp.json and checks every limit.
func compose(l listing, n notice, public *url.URL, notes string) (pluginJSON, mcpJSON, error) {
	base := public.String()
	long := strings.TrimSpace(l.Description) + "\n\n" + n.Notice + "\n\n" + n.creditText()
	repo := strings.TrimSuffix(strings.TrimSuffix(n.Help.URL, "/"), "/issues")

	var p pluginJSON
	var m mcpJSON
	switch {
	case !nameRule.MatchString(l.Name):
		return p, m, fmt.Errorf("listing: name %q must be lowercase letters and digits joined by single hyphens", l.Name)
	case !versionRule.MatchString(l.Version):
		return p, m, fmt.Errorf("listing: version %q must be MAJOR.MINOR.PATCH", l.Version)
	case l.DisplayName == "" || utf8.RuneCountInString(l.DisplayName) > maxNameChars:
		return p, m, fmt.Errorf("listing: displayName %q must be 1 to %d characters", l.DisplayName, maxNameChars)
	case l.ShortDescription == "" || utf8.RuneCountInString(l.ShortDescription) > maxNameChars:
		return p, m, fmt.Errorf("listing: shortDescription %q must be 1 to %d characters", l.ShortDescription, maxNameChars)
	case strings.TrimSpace(l.Description) == "":
		return p, m, errors.New("listing: description is empty")
	case utf8.RuneCountInString(long) > maxDescriptionChars:
		return p, m, fmt.Errorf("listing: long description (description, notice and credit) is %d characters, over %d", utf8.RuneCountInString(long), maxDescriptionChars)
	case l.DeveloperName == "":
		return p, m, errors.New("listing: developerName is empty")
	case l.Category == "":
		return p, m, errors.New("listing: category is empty")
	case len(l.DefaultPrompt) > maxPrompts:
		return p, m, fmt.Errorf("listing: %d starter prompts (defaultPrompt), at most %d", len(l.DefaultPrompt), maxPrompts)
	case l.BrandColor != "" && !colourRule.MatchString(l.BrandColor):
		return p, m, fmt.Errorf("listing: brandColor %q must be #rrggbb", l.BrandColor)
	}
	for i, pr := range l.DefaultPrompt {
		if pr == "" || utf8.RuneCountInString(pr) > maxPromptChars {
			return p, m, fmt.Errorf("listing: starter prompt %d must be 1 to %d characters", i+1, maxPromptChars)
		}
	}
	tc := l.Review.TestCases
	if len(tc.Positive) != positiveCases {
		return p, m, fmt.Errorf("listing: %d positive test cases, need exactly %d", len(tc.Positive), positiveCases)
	}
	for i, c := range tc.Positive {
		if c.Description == "" || c.Prompt == "" || c.ToolsTriggered == "" || c.ExpectedBehavior == "" {
			return p, m, fmt.Errorf("listing: positive test case %d needs description, prompt, tools_triggered and expected_behavior", i+1)
		}
	}
	if len(tc.Negative) != negativeCases {
		return p, m, fmt.Errorf("listing: %d negative test cases, need exactly %d", len(tc.Negative), negativeCases)
	}
	for i, c := range tc.Negative {
		if c.Description == "" || c.Prompt == "" {
			return p, m, fmt.Errorf("listing: negative test case %d needs description and prompt", i+1)
		}
	}

	in := iface{
		DisplayName:       l.DisplayName,
		ShortDescription:  l.ShortDescription,
		LongDescription:   long,
		DeveloperName:     l.DeveloperName,
		Category:          l.Category,
		Capabilities:      l.Capabilities,
		WebsiteURL:        base,
		PrivacyPolicyURL:  base + "/privacy",
		TermsOfServiceURL: base + "/terms",
		SupportURL:        n.Help.URL,
		DefaultPrompt:     l.DefaultPrompt,
		BrandColor:        l.BrandColor,
		ComposerIcon:      "./" + iconPath,
		Logo:              "./" + iconPath,
	}
	links := map[string]string{
		"websiteURL": in.WebsiteURL, "privacyPolicyURL": in.PrivacyPolicyURL, "termsOfServiceURL": in.TermsOfServiceURL,
		"supportURL": in.SupportURL, "repository": repo, "mcp url": base + "/mcp",
	}
	if l.Review.DemoRecordingURL != "" {
		links[demoRecordingPath] = l.Review.DemoRecordingURL
	}
	names := make([]string, 0, len(links))
	for k := range links {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if err := checkLink(k, links[k]); err != nil {
			return p, m, err
		}
	}

	p = pluginJSON{
		Schema:      "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		Name:        l.Name,
		Version:     l.Version,
		Description: l.ShortDescription,
		Homepage:    base,
		Repository:  repo,
		Extensions: pluginExtensions{OpenAI: openAIExtension{
			Interface:   in,
			Review:      l.Review,
			Publication: publication{ReleaseNotes: notes},
		}},
	}
	srv := mcpServer{Type: "streamable-http", URL: base + "/mcp"}
	srv.Extensions.OpenAI.Auth.Type = "oauth"
	srv.Extensions.OpenAI.Auth.Client.Mode = "dcr"
	m = mcpJSON{
		Schema:     "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		MCPServers: map[string]mcpServer{l.Name: srv},
	}
	return p, m, nil
}

// checkLink refuses a link that is not an https URL of at most 1024
// characters.
func checkLink(name, link string) error {
	u, err := url.Parse(link)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("link: %s %q must be an https URL", name, link)
	}
	if len(link) > maxURLChars {
		return fmt.Errorf("link: %s is %d characters, over %d", name, len(link), maxURLChars)
	}
	return nil
}

type zipEntry struct {
	name string
	data []byte
}

// writeZip writes entries in order with a fixed modification time, so the
// same sources give the same bytes.
func writeZip(entries []zipEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: zip.Deflate, Modified: fixedTime})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
