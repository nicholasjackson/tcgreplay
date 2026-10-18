---
tags: [skills, testing, build, conventions]
---

# Skill text and packaging

- Never write "Claude" or "Nic " in a `skills/tcg-*` skill. Skills serve any assistant and any player, and `skills/skills_test.go` fails on either word.
- Never exclude `skills/tcg-*` from a build context (`.dockerignore` or other packaging). `skills/skills.go` embeds them with `//go:embed tcg-*`, so the binary will not build without them.
