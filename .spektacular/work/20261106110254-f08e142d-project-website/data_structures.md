**Go: the site package.** One exported constructor returns the embedded site as an `http.Handler`. The hosted router mounts it at every path it doesn't route elsewhere.

```go
package site

// Handler serves the embedded website: pages, build assets and the site's
// 404 page. It never sets cookies and needs no sign-in. Built returns false
// when only the placeholder is embedded (the site was not built).
func Handler() http.Handler
func Built() bool
```

**Go: hosted routes (a contract, not a type).** The path table the hosted router guarantees:

| Path | Served by | Sign-in |
|---|---|---|
| `/mcp` | MCP (unchanged) | access token |
| `/auth/*`, OAuth metadata, `/register`, `/authorize`, `/token`, `/healthz` | sign-in service (unchanged) | none |
| `/admin`, `/admin/*` | cache admin (unchanged) | admin |
| `/board` (`/board/` redirects to it) | board front door, path rewritten to `/` | session |
| `/assets/*`, `/ws`, `/api/*` | board front door (unchanged paths) | session |
| everything else (`/`, `/docs/…`, `/privacy`, `/_astro/*`, `/logo.png`, …) | site package | none |

**Board link format (changed).** `<public>/board?board=<handle>` replaces `<public>/?board=<handle>`. The handle itself is unchanged.

**Website build: facts available to pages.** Produced by the build-time loader. Pages read these values instead of hard-coding them.

```ts
type Workflow = { name: string; description: string; order: number }  // from internal/guides/*.md frontmatter + guide order
type Limits   = { requestsPerMinute: number; maxUsers: number; idleTimeout: string /* e.g. "60 minutes" */ }  // from .do/app.yaml
type Example  = { log: string /* anonymised battle log */; deck: string /* TCG Live export */ }
const site    = { publicURL: "https://tcgreplay.demo.gs", mcpURL: "https://tcgreplay.demo.gs/mcp", repoURL: "https://github.com/nicholasjackson/tcgreplay" }
```

**Website content: a workflow page.** A Markdown file per guide, whose frontmatter names the guide it documents. The build pairs the pages with the guides one to one.

```yaml
---
guide: review-game          # must equal a guide's name
title: Review a game
prompts:                    # at least one; each gets a copy button
  - "Review this game with me. Start with the turn where I lost tempo."
---
```

**Example game files.** `site/src/content/example/battle-log.txt` is the corpus log with "GatoruSoju" → "Rival". The decklist is read from `testdata/decks/mega-lucario-ex-v12.txt` and not copied.
