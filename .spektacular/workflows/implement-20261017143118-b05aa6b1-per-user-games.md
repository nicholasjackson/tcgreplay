# Working context: implement 20261017143118-b05aa6b1-per-user-games (orchestrated)

Code root (worktree): /home/nicj/code/github.com/nicholasjackson/tcgreplay/.spektacular/worktrees/20261017143118-b05aa6b1-per-user-games/tcgreplay
Run spektacular only from the project root.

## read_plan decisions
- Structure OK; spec coverage complete; no Changelog section yet (first-task mode).
- Drift (minor, adapted per plan's own Open assumptions — option 2, no hand-back):
  - google-login's cap is not an `admit` in limit.go: it is `store.addSession(now, acct, maxUsers)` called from `Auth.signIn` at the Google callback; `busyPage(w, maxUsers)` takes the number. Places.Admit goes in `signIn`; Release goes where `releaseLocked` drops an account to zero live sign-ins.
  - Cap tests live in `internal/hostauth/cap_test.go` and `cmd/tcgreplay/hosted_cap_test.go` (not limit_test.go).
  - Game has no Close (memory-only); "close" = drop the pointer. Hub's `watched` map keeps old games forever: reset it on watch of a new game / on clear so expiry really frees memory.
- Orchestrator: human task (apply app definition) → hand back QUESTION with console steps (app was created by hand in the console); don't run wrap-up or commit until answered.
