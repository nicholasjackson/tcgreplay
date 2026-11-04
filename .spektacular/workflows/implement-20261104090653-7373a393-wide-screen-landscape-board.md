# Working context: 20261104090653-7373a393-wide-screen-landscape-board (orchestrated)

- read_plan: structure OK, no drift found (all referenced files/symbols exist; startReplay lives in lib/playback.svelte.js), spec fully covered, no Changelog yet (first-task mode).
- Open question carried: if a benched card at 2560x1440 (timeline beside) can't reach >=191px without cramping, hand back QUESTION.
- Another session edits the main checkout concurrently (Go server, maybe Hand.svelte). Work only in the worktree.
- analyze (task 1, Hand counts): Hand.svelte .count is absolute right:12px; plan: `.hand:not(.phone) .count` in-flow flex item. Phone keeps absolute count.
- BLOCKER at implement step: `implement new` created NO git worktree (git worktree list shows only main; no spek/ branch) although config has implement.worktrees: true; instructions point at repo root = main checkout, where another session is editing. Handed back as QUESTION before writing any code.
- ANSWER (orchestrator): option 1. Worktree created manually: /home/nicj/code/github.com/nicholasjackson/tcgreplay-wide-board on branch spek/20261104090653-7373a393-wide-screen-landscape-board (from main 115126b). All code/build/e2e there; spektacular from project root. Port 7790 is ours. At merge step: STOP before merging, report the branch (user merges by hand). No commits unless a step says.
- Baseline: full suite 217 passed/62 skipped/0 failed after task 1 (harness 'dialog fails the test' shows ✘ but is test.fail, expected).
- goal check: CLI compares staged diff against the MAIN checkout (registry root), so it warns 'diff changed' (other session's edits). Expected; stage the worktree diff anyway.
- Task 1 done (Hand counts). Scripts: scratchpad tick.py (tick plan boxes) and clog.py (append changelog entry).
- Tasks 2,3 done. Milestone 1 complete. Height (not width) likely binds the 1.6x target at 2560x1440: ~1397px tall / 191 = 7.3cw for 4 rows+peek+names.
- Task 4 done (wide flag, wide profile). Next: Landscape rows & sizing (High) - tests must be written by a sub-agent.
