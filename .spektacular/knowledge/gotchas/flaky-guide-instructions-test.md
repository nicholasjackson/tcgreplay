---
tags: [testing, mcp, concurrency]
---

# TestGuideInstructions flakes under -race (pre-existing)

`go test -race ./internal/mcpserver/` fails now and then in `TestGuideInstructions` (both the desktop and hosted subtests, in-memory transport) with:

```
testing.go:1619: TempDir RemoveAll cleanup: unlinkat /tmp/TestGuideInstructions…/001: directory not empty
```

The test itself passes. The failure is the cleanup of the `t.TempDir()` the test's game manager uses: something keeps writing into that directory after the test function returns, so `RemoveAll` races it. Measured on 2026-11-07: 8 failures in 30 runs on main without any feature work, 3 in 30 on a feature branch. Plain `go test` (no `-race`) has not shown it.

- When verifying a change, a lone failure of this shape in `internal/mcpserver` is this known flake, not a regression. Re-run, or run the test with `-count=30` on main to compare.
- The likely fix is to stop or wait for the manager's background writer (a save, a card warm or similar) before the test ends, for example with a `t.Cleanup` that closes the manager. Not yet investigated.

Related: [[test-card-swaps-revert]].
