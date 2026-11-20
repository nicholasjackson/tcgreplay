---
created_date: "2026-11-04"
document_status: final
closed_date: "2026-11-04"
---

# Test plan: 20261104082020-109dceb1-tcgdex-spaces-cache

These checks need a real DigitalOcean Spaces bucket and the hosted deployment, so the automated suite cannot run them. The suite covers the rest against an in-process stand-in for Spaces (`internal/spaces/spacestest`) and a fake TCGdex. Run them in the order below, after the code is deployed.

## 0. Prerequisite: create the Spaces bucket and set its secrets (outstanding human task)

The plan's human task "Create the Spaces bucket and set its secrets" was skipped by the agent run and is still open. Until it is done, the hosted playmat runs with a local-only card cache.

- **Who / when:** the operator, before any check below, in the DigitalOcean control panel.
- **How:**
  1. Create a Spaces bucket near the app (App Platform region `lon`, so the nearest Spaces region, e.g. `ams3` or `fra1`). Keep file listing restricted (private). The CDN is not needed.
  2. Create a Spaces access key limited to that bucket, with read and write ("Limited access" key). Note the key id and secret.
  3. On the App Platform component `playmat`, set `TCGREPLAY_SPACES_ENDPOINT` (`https://<region>.digitaloceanspaces.com`), `TCGREPLAY_SPACES_BUCKET`, `TCGREPLAY_SPACES_KEY` and `TCGREPLAY_SPACES_SECRET`, all encrypted. `doctl apps update` does not carry secret values, so set them in the console. Save to redeploy.
- **Expected result (the task's acceptance criteria):**
  - a private bucket exists, with a key that can read and write it;
  - the four variables are set on `playmat`, the key and secret encrypted;
  - the new deployment's runtime log shows `card cache: /home/nonroot/.cache/tcgreplay backed by shared store https://<region>.digitaloceanspaces.com/<bucket>`, then `card cache: catalog warmed in …`;
  - the log has no `warning: card cache` line and no `shared store unavailable` line.

  When all of these hold, tick the task in the plan.

## 1. Success metric: zero card-source requests after a redeploy (real Spaces)

The behavioural tests cover this metric (`internal/cards/restart_test.go` `TestWarmRestartMakesNoCardSourceRequests`, `internal/game/spacescache_test.go`). This end-to-end check confirms it on real Spaces.

- **What to measure:** card-data requests to TCGdex during the first import after a redeploy, for a game imported before it. Threshold: 0.
- **How:**
  1. With step 0 done, import a saved TCG Live battle log on the hosted playmat (`import_log` from claude.ai).
  2. In the Spaces console, check that the bucket holds `tcgdex/sets.json`, `tcgdex/sets/<id>.json` and `tcgdex/cards/<id>.json`. Download one object and check it is the envelope `{"fetched":"<RFC 3339>","data":{…}}`, private, with `Content-Type: application/json`.
  3. Redeploy (Actions → Force rebuild and deploy, or push a no-op commit to `main`).
  4. Import the same log again. Watch the runtime log, or compare object `Last-Modified` times in the bucket before and after.
- **Expected result:**
  - The start-up log names the shared store.
  - The second import gives the same cards on the board.
  - No `tcgdex/cards/*` or `tcgdex/sets/*` object used by the game gets a newer `Last-Modified` from the second import. A card-source fetch would rewrite them; only entries past their refresh age may be refreshed.
- **Who / when:** operator, once after step 0, then after any change to `internal/cards` or `internal/spaces`.

## 2. Success metric: first import after a restart within 2 seconds of a warm import

- **What to measure:** the first import after a restart, other than the first deploy, must finish within 2 s of a warm-cache import of the same game.
- **How:**
  1. With step 0 done and the bucket already filled (step 1), import a reference battle log twice in a row on a running instance. Time the second import from sending `import_log` to its result. This is the warm import.
  2. Redeploy. Wait for `card cache: catalog warmed in <d>` in the runtime log and note `<d>`.
  3. Import the same log once. Time it the same way. This is the first import after the restart.
  4. Also try importing as soon as `/healthz` answers, before the warm line appears, and note that time.
- **Expected result:**
  - The time from step 3 minus the time from step 1 is at most 2 s.
  - The catalog warm duration from step 2 is recorded alongside, for reference.
  - If step 4 is over 2 s but step 3 is within it, the warm is doing its job. Record both.
- **Who / when:** operator, after step 0 and step 1, against production.

## 3. Manual review: real Spaces failure modes

- **What to check:** the hosted playmat keeps working when the shared store is wrong or unreachable, and logs the failure once.
- **How:**
  1. **Wrong secret.** Set `TCGREPLAY_SPACES_SECRET` to a wrong value and redeploy. Sign in, import a game and open the board.
  2. **Unreachable endpoint.** Restore the secret. Set `TCGREPLAY_SPACES_ENDPOINT` to an unreachable https endpoint (e.g. `https://xxx1.digitaloceanspaces.com`) and redeploy. Repeat the import.
  3. Restore the real values and redeploy.
- **Expected result:**
  - In both bad cases the playmat starts, `/healthz` answers, the import succeeds, cards show text and art, and no player sees an error.
  - The runtime log has one `card cache: shared store unavailable (… <code or error> …); using the local cache and the card source, retrying in 1m0s` line per outage minute, not one per lookup. The wrong-secret case names `SignatureDoesNotMatch` (or `InvalidAccessKeyId`).
  - After restoring, the log shows the shared store again and no warning.
- **Who / when:** operator, once after step 0.

## 4. Manual review: a missing key is a 404, not a breaker trip (open assumption)

- **What to check:** real Spaces answers a GET for a missing object with 404 for the bucket-limited key. The tiered store treats that as a normal miss. A 403 would instead trip the one-minute breaker on every cold miss.
- **How:** with step 0 done and a fresh bucket, or one with an object removed, import a game that needs a card not yet in the bucket. Read the runtime log.
- **Expected result:** no `shared store unavailable` line during the import, and the card's object appears in the bucket afterwards. If the line appears with `AccessDenied` for missing keys, stop. Ask whether to require a key with list permission (documentation only) or to treat a 403 on GET as a miss, as the plan's open question says.
- **Who / when:** operator, during step 0 and step 1.

## 5. Manual review: no settings and desktop builds are unchanged

- **What to check:** without the shared store, behaviour is as before.
- **How:**
  1. Run the container locally with no Spaces variables: `make docker-build`, then `make docker-run`, with only the Google credentials exported. Read its start-up log.
  2. Run a desktop build (`tcgreplay` stdio, or `make dev`) with the four `TCGREPLAY_SPACES_*` variables exported in the shell. Import a game.
- **Expected result:**
  - The container logs exactly one `card cache: local only, in … (no TCGREPLAY_SPACES_* settings)` line and no `warning`.
  - The desktop build logs nothing about the card cache or a shared store, makes no Spaces requests (the bucket's object times do not change), and imports as before from `~/.cache/tcgreplay`.
- **Who / when:** developer or operator, pre-release.
