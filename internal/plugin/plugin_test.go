package plugin

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"os"
	"strings"
	"testing"
)

// realSources reads the repository's own sources for the package.
func realSources(t *testing.T) Sources {
	t.Helper()
	read := func(p string) []byte {
		b, err := os.ReadFile("../../" + p)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	return Sources{
		AppSpec:      string(read(".do/app.yaml")),
		Notice:       string(read("internal/notice/notice.json")),
		Listing:      string(read("plugin/listing.json")),
		ReleaseNotes: string(read("plugin/release-notes.md")),
		Icon:         read("plugin/assets/icon.png"),
		Skills:       map[string]string{"tcgreplay-coach": string(read("plugin/skills/tcgreplay-coach/SKILL.md"))},
	}
}

// coachSkill is the real skill with edit applied to its text.
func coachSkill(t *testing.T, edit func(string) string) map[string]string {
	t.Helper()
	return map[string]string{"tcgreplay-coach": edit(realSources(t).Skills["tcgreplay-coach"])}
}

// unzip returns the package's entries in order, and their contents.
func unzip(t *testing.T, b []byte) ([]string, map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, f.Name)
		files[f.Name] = data
	}
	return names, files
}

// builtPlugin is the parts of plugin.json the tests read.
type builtPlugin struct {
	Name       string            `json:"name"`
	Author     map[string]string `json:"author"`
	Extensions struct {
		OpenAI struct {
			Interface struct {
				DisplayName       string   `json:"displayName"`
				ShortDescription  string   `json:"shortDescription"`
				LongDescription   string   `json:"longDescription"`
				WebsiteURL        string   `json:"websiteURL"`
				PrivacyPolicyURL  string   `json:"privacyPolicyURL"`
				TermsOfServiceURL string   `json:"termsOfServiceURL"`
				SupportURL        string   `json:"supportURL"`
				DefaultPrompt     []string `json:"defaultPrompt"`
				ComposerIcon      string   `json:"composerIcon"`
				Logo              string   `json:"logo"`
			} `json:"interface"`
			Review struct {
				TestCases struct {
					Positive []map[string]string `json:"positive"`
					Negative []map[string]string `json:"negative"`
				} `json:"test_cases"`
			} `json:"review"`
			Publication struct {
				ReleaseNotes string `json:"release_notes"`
			} `json:"publication"`
		} `json:"com.openai"`
	} `json:"extensions"`
}

// The shared notice and credit, word for word from internal/notice/notice.json.
const (
	wantNotice = "tcgreplay is an unofficial fan project. It is not affiliated with, endorsed, sponsored or approved by Nintendo, The Pokémon Company, Creatures or Game Freak. Pokémon and its trademarks are theirs."
	wantCredit = "Card data and images from TCGdex (https://tcgdex.dev) and malie.io (https://malie.io)."
)

// TestBuildFromRealSources: built from the repository's sources, the
// package holds exactly plugin.json, mcp.json, the icon and the skill,
// at skills/tcgreplay-coach/SKILL.md unchanged. plugin.json
// shows "TCG Replay", a long description ending with the shared notice and
// credit, links built from the App Platform address and the notice's help
// link, five positive and three negative test cases and the release notes;
// mcp.json connects to https://tcgreplay.app/mcp over streamable HTTP with
// OAuth through dynamic registration.
func TestBuildFromRealSources(t *testing.T) {
	src := realSources(t)
	b, err := Build(src)
	if err != nil {
		t.Fatal(err)
	}
	names, files := unzip(t, b)
	if strings.Join(names, ",") != "plugin.json,mcp.json,assets/icon.png,skills/tcgreplay-coach/SKILL.md" {
		t.Fatalf("package entries %v, want plugin.json, mcp.json, assets/icon.png, skills/tcgreplay-coach/SKILL.md", names)
	}
	if !bytes.Equal(files["assets/icon.png"], src.Icon) {
		t.Error("package icon differs from plugin/assets/icon.png")
	}
	if string(files["skills/tcgreplay-coach/SKILL.md"]) != src.Skills["tcgreplay-coach"] {
		t.Error("package skill differs from plugin/skills/tcgreplay-coach/SKILL.md")
	}
	if bytes.Contains(files["plugin.json"], []byte(`"skills"`)) {
		t.Error("plugin.json has a skills field; the schema forbids it and OpenAI discovers skills/")
	}

	var p builtPlugin
	if err := json.Unmarshal(files["plugin.json"], &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Author) != 1 || p.Author["name"] != "Nic Jackson" {
		t.Errorf("author %v, want only the name Nic Jackson (no email)", p.Author)
	}
	in := p.Extensions.OpenAI.Interface
	if p.Name != "tcgreplay" || in.DisplayName != "TCG Replay" {
		t.Errorf("name %q, displayName %q; want tcgreplay, TCG Replay", p.Name, in.DisplayName)
	}
	if !strings.HasSuffix(in.LongDescription, "\n\n"+wantNotice+"\n\n"+wantCredit) {
		t.Errorf("long description does not end with the notice and credit:\n%s", in.LongDescription)
	}
	for name, got := range map[string][2]string{
		"websiteURL":        {in.WebsiteURL, "https://tcgreplay.app"},
		"privacyPolicyURL":  {in.PrivacyPolicyURL, "https://tcgreplay.app/privacy"},
		"termsOfServiceURL": {in.TermsOfServiceURL, "https://tcgreplay.app/terms"},
		"supportURL":        {in.SupportURL, "https://github.com/nicholasjackson/tcgreplay/issues"},
		"composerIcon":      {in.ComposerIcon, "./assets/icon.png"},
		"logo":              {in.Logo, "./assets/icon.png"},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %q, want %q", name, got[0], got[1])
		}
	}
	tc := p.Extensions.OpenAI.Review.TestCases
	if len(tc.Positive) != 5 || len(tc.Negative) != 3 {
		t.Errorf("%d positive and %d negative test cases, want 5 and 3", len(tc.Positive), len(tc.Negative))
	}
	for i, c := range tc.Positive {
		for _, f := range []string{"description", "prompt", "tools_triggered", "expected_behavior"} {
			if c[f] == "" {
				t.Errorf("positive case %d has no %s", i+1, f)
			}
		}
	}
	for i, c := range tc.Negative {
		for _, f := range []string{"description", "prompt"} {
			if c[f] == "" {
				t.Errorf("negative case %d has no %s", i+1, f)
			}
		}
	}
	if p.Extensions.OpenAI.Publication.ReleaseNotes != strings.TrimSpace(src.ReleaseNotes) {
		t.Error("release notes are not plugin/release-notes.md")
	}

	var m struct {
		MCPServers map[string]struct {
			Type       string `json:"type"`
			URL        string `json:"url"`
			Extensions struct {
				OpenAI struct {
					Auth struct {
						Type   string `json:"type"`
						Client struct {
							Mode string `json:"mode"`
						} `json:"client"`
					} `json:"auth"`
				} `json:"com.openai"`
			} `json:"extensions"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(files["mcp.json"], &m); err != nil {
		t.Fatal(err)
	}
	s, ok := m.MCPServers["tcgreplay"]
	if len(m.MCPServers) != 1 || !ok {
		t.Fatalf("mcp.json servers %v, want one named tcgreplay", m.MCPServers)
	}
	if s.Type != "streamable-http" || s.URL != "https://tcgreplay.app/mcp" || s.Extensions.OpenAI.Auth.Type != "oauth" || s.Extensions.OpenAI.Auth.Client.Mode != "dcr" {
		t.Errorf("mcp.json server %+v, want streamable-http at https://tcgreplay.app/mcp with oauth dcr", s)
	}
}

// TestBuildIsReproducible: two builds from the same sources are
// byte-identical.
func TestBuildIsReproducible(t *testing.T) {
	a, err := Build(realSources(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build(realSources(t))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Error("two builds from the same sources differ")
	}
}

// TestListingSourceLeavesDerivedPartsToTheBuilder: the real listing
// source spells out no address, notice or support link: they come from
// .do/app.yaml and internal/notice/notice.json.
func TestListingSourceLeavesDerivedPartsToTheBuilder(t *testing.T) {
	l := realSources(t).Listing
	for _, s := range []string{"http://", "https://", "tcgreplay.app", wantNotice, "unofficial fan project", "github.com", "tcgdex.dev", "malie.io"} {
		if strings.Contains(l, s) {
			t.Errorf("plugin/listing.json spells out %q", s)
		}
	}
}

// editListing returns the real listing source with edit applied.
func editListing(t *testing.T, edit func(l map[string]any)) string {
	t.Helper()
	var l map[string]any
	if err := json.Unmarshal([]byte(realSources(t).Listing), &l); err != nil {
		t.Fatal(err)
	}
	edit(l)
	b, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// cases returns the listing's positive or negative test cases.
func cases(l map[string]any, kind string) []any {
	return l["review"].(map[string]any)["test_cases"].(map[string]any)[kind].([]any)
}

func setCases(l map[string]any, kind string, c []any) {
	l["review"].(map[string]any)["test_cases"].(map[string]any)[kind] = c
}

// pngOf returns a w by h PNG.
func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestBuildRefusesBrokenSources: each source that breaks one of OpenAI's
// rules is refused with a message naming the rule.
func TestBuildRefusesBrokenSources(t *testing.T) {
	for _, tt := range []struct {
		name   string
		break_ func(t *testing.T, s *Sources)
		want   string
	}{
		{"display name over 30", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["displayName"] = strings.Repeat("x", 31) })
		}, "displayName"},
		{"subtitle over 30", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["shortDescription"] = strings.Repeat("x", 31) })
		}, "shortDescription"},
		{"description over 4000", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["description"] = strings.Repeat("x", 4000) })
		}, "over 4000"},
		{"four starter prompts", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["defaultPrompt"] = []any{"a", "b", "c", "d"} })
		}, "starter prompts"},
		{"starter prompt over 128", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["defaultPrompt"] = []any{strings.Repeat("x", 129)} })
		}, "starter prompt 1"},
		{"four positive cases", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { setCases(l, "positive", cases(l, "positive")[:4]) })
		}, "positive test cases"},
		{"positive case without expected behaviour", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { delete(cases(l, "positive")[2].(map[string]any), "expected_behavior") })
		}, "positive test case 3"},
		{"two negative cases", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { setCases(l, "negative", cases(l, "negative")[:2]) })
		}, "negative test cases"},
		{"http demo video link", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) {
				l["review"].(map[string]any)["demo_recording_url"] = "http://video.example/demo"
			})
		}, "must be an https URL"},
		{"http public address", func(t *testing.T, s *Sources) {
			s.AppSpec = strings.Replace(s.AppSpec, "value: https://tcgreplay.app", "value: http://tcgreplay.app", 1)
		}, "TCGREPLAY_PUBLIC_URL"},
		{"non-square icon", func(t *testing.T, s *Sources) { s.Icon = pngOf(t, 512, 256) }, "must be square"},
		{"tiny icon", func(t *testing.T, s *Sources) { s.Icon = pngOf(t, 32, 32) }, "48 to 4096"},
		{"icon not a PNG", func(t *testing.T, s *Sources) { s.Icon = []byte("<svg/>") }, "not a PNG"},
		{"missing notice file", func(t *testing.T, s *Sources) { s.Notice = "" }, "notice.json is missing"},
		{"test credentials", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["review"].(map[string]any)["test_credentials"] = map[string]any{"user": "x"} })
		}, "test_credentials"},
		{"author email", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["email"] = "someone@example.com" })
		}, "email"},
		{"client secret", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["clientSecret"] = "x" })
		}, "clientSecret"},
		{"hooks", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["hooks"] = []any{} })
		}, "hooks"},
		{"literal address", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) {
				cases(l, "positive")[0].(map[string]any)["prompt"] = "Copy the log from https://tcgreplay.app/docs/example"
			})
		}, "spells out an address"},
		{"notice in the listing", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["description"] = "A playmat. " + wantNotice })
		}, "fan-project notice"},
		{"support link in the listing", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) {
				l["description"] = "Report bugs at github.com/nicholasjackson/tcgreplay/issues."
			})
		}, "support link"},
		{"unknown placeholder", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { cases(l, "positive")[0].(map[string]any)["prompt"] = "See {website}/docs" })
		}, "unknown placeholder"},
		{"bad package name", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["name"] = "TCG--Replay" })
		}, "name"},
		{"unknown field", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["subtitle"] = "x" })
		}, "subtitle"},
		{"category OpenAI does not list", func(t *testing.T, s *Sources) {
			s.Listing = editListing(t, func(l map[string]any) { l["category"] = "Games" })
		}, "category"},
		{"no release notes", func(t *testing.T, s *Sources) { s.ReleaseNotes = "  \n" }, "release notes"},
		{"no skill", func(t *testing.T, s *Sources) { s.Skills = nil }, "at least one"},
		{"skill without frontmatter", func(t *testing.T, s *Sources) {
			s.Skills = map[string]string{"tcgreplay-coach": "# Just instructions\n"}
		}, "no frontmatter"},
		{"skill name not its directory", func(t *testing.T, s *Sources) {
			s.Skills = coachSkill(t, func(x string) string { return strings.Replace(x, "name: tcgreplay-coach", "name: coach", 1) })
		}, "must match its directory"},
		{"skill directory name", func(t *testing.T, s *Sources) {
			s.Skills = map[string]string{"TCG_Coach": "---\nname: TCG_Coach\ndescription: x\n---\nbody\n"}
		}, "directory name"},
		{"skill identity over 64", func(t *testing.T, s *Sources) {
			n := strings.Repeat("x", 60)
			s.Skills = map[string]string{n: "---\nname: " + n + "\ndescription: x\n---\nbody\n"}
		}, "skill identity"},
		{"skill without description", func(t *testing.T, s *Sources) {
			s.Skills = map[string]string{"coach": "---\nname: coach\n---\nbody\n"}
		}, "no description"},
		{"skill description over 1024", func(t *testing.T, s *Sources) {
			s.Skills = map[string]string{"coach": "---\nname: coach\ndescription: " + strings.Repeat("x", 1025) + "\n---\nbody\n"}
		}, "over 1024"},
		{"skill without instructions", func(t *testing.T, s *Sources) {
			s.Skills = map[string]string{"coach": "---\nname: coach\ndescription: x\n---\n\n"}
		}, "no instructions"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := realSources(t)
			tt.break_(t, &src)
			_, err := Build(src)
			if err == nil {
				t.Fatalf("Build accepted the broken source")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Build error %q does not name %q", err, tt.want)
			}
		})
	}
}

// TestPublicPlaceholderFilled: {public} in the listing source becomes the
// App Platform address in the package.
func TestPublicPlaceholderFilled(t *testing.T) {
	src := realSources(t)
	src.Listing = editListing(t, func(l map[string]any) {
		cases(l, "positive")[0].(map[string]any)["prompt"] = "Copy the decklist and battle log from {public}/docs/example"
	})
	b, err := Build(src)
	if err != nil {
		t.Fatal(err)
	}
	_, files := unzip(t, b)
	var p builtPlugin
	if err := json.Unmarshal(files["plugin.json"], &p); err != nil {
		t.Fatal(err)
	}
	if got := p.Extensions.OpenAI.Review.TestCases.Positive[0]["prompt"]; got != "Copy the decklist and battle log from https://tcgreplay.app/docs/example" {
		t.Errorf("prompt %q, want the placeholder filled with https://tcgreplay.app", got)
	}
}
