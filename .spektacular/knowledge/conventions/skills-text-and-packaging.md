---
tags: [guides, testing, build, conventions]
---

# Guide text and packaging

- Never write "Claude" or "Nic " in an `internal/guides` guide. Guides serve any assistant and any player, and `internal/guides/guides_test.go` fails on either word.
- Never exclude `internal/guides` from a build context (`.dockerignore` or other packaging). `internal/guides/guides.go` embeds the guides with `//go:embed *.md`, so the binary will not build without them.
