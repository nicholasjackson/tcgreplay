# Working context: public-launch epic, spec launch-docs

## Why
"Update the docs on using it" (user, at the start of the launch work). Last spec of the public-launch epic: the connection docs for the ChatGPT Plugin Directory listing and a note on refreshing the plugin after an update (OpenAI holds tool changes for review; users may need to refresh the connection to see new tools).

## Epic state (2026-11-12)
domain-move; observability -> privacy-terms-disclaimer -> visitor-analytics; chatgpt-submission-prep depends on privacy + domain-move. All written. domain-move already changes every demo.gs address in the docs; this spec doesn't repeat that.

## Facts
- site/src/content/docs/connect.md has sections: claude.ai (custom connector), ChatGPT (custom MCP server: Plugins → Add → Add custom MCP server, OAuth), Claude Code (claude mcp add), Claude desktop app, Next. Addresses still say tcgreplay.demo.gs.
- Other docs: desktop, developers, import, rewind, sign-in, workflows; limits page; privacy page.
- Listing name will be "TCG Replay"; support = GitHub issues.
- docs interview: both ChatGPT paths (directory first once listed); What's new page + refresh steps. Added docs index (/docs/ 404) as my addition.
- launch-docs spec finished and written (2026-11-12). Proposed depends_on: domain-move, chatgpt-submission-prep (ask user).
- launch-docs depends_on domain-move recorded (2026-11-12). Epic fully specified; next: /spek-plan-epic.

## Plan-epic run (2026-11-12)
- Orchestrating /spek-plan-epic for 20261111093348-e1b5c2b4-public-launch. Root: /home/nicj/code/github.com/nicholasjackson/tcgreplay
- Started children: domain-move, observability (both ready at start).
- QUESTION observability (discovery): DO doesn't keep runtime logs across redeploys; options A managed OpenSearch forwarding (rec), B live viewer only + soften AC, C daily counts in Spaces. Awaiting user.
- User answer (observability): pointed at DigitalOcean Insights (https://cloud.digitalocean.com/insights?i=ea5670) as the place to read usage/logs. Sent to child to research and carry on.
- QUESTION 2 observability: public docs say Insights has App Platform metrics+alerts but no App Platform logs, no custom telemetry. Asked user whether playmat log lines show in Insights Logs (A plan on Insights logs / B Insights + Managed OpenSearch (rec if not) / C Insights + live viewer, soften AC).
- User answer 2 (observability): A — playmat log lines DO appear in Insights Logs; plan around Insights logs + metrics.
- DONE domain-move (summary kept in scratchpad done-domain-move.md). Flags overlap with observability on cmd/tcgreplay/hosted.go + .do/app.yaml.
- Started child: launch-docs (ready after domain-move).
- DONE observability (summary in scratchpad done-observability.md). Started child: privacy-terms-disclaimer.
- DONE launch-docs (summary in scratchpad done-launch-docs.md).
- DONE privacy-terms-disclaimer (summary in scratchpad done-privacy.md). Started children: visitor-analytics, chatgpt-submission-prep.
- DONE visitor-analytics (summary in scratchpad done-visitor-analytics.md). Cross-plan: __cf_bm edge cookie vs privacy 'no cookies' wording.
- DONE chatgpt-submission-prep (summary in scratchpad done-chatgpt.md). All 6 planned; now Step 6 (cross-plan decisions).
- Step 6: put 4 cross-plan decisions to user (cookie sentence, visits retention test, request Host rule, What's new for launch specs).
- User accepted all four decisions; applying to plans.
- Decisions applied to privacy (plan+context) and domain-move (plan+context); chatgpt already matched D4. Next: write summary sections.
- Summary written (6 sections + decisions). epic order added 5 deps: observability<-domain-move; chatgpt<-visitor-analytics; launch-docs<-privacy, visitor-analytics, chatgpt. Next: Step 7 review.
- Step 7 review presented to user; awaiting changes/confirmation.
- Review confirmed by user with no changes (2026-11-12). Epic fully planned; next: implement this epic.
- User chose to implement the whole public-launch epic (via spek-implement-epic) instead of one spec.

## Implement-epic run (2026-11-12)
- Orchestrating /spek-implement-epic for 20261111093348-e1b5c2b4-public-launch. Tree clean (app.yaml edits already committed). Linear chain: domain-move → observability → privacy → visitor-analytics → chatgpt → launch-docs.
- Started child: domain-move.
- QUESTION domain-move: 5 agent tasks done; human task c2f0b060 (console steps per docs/domain-move.md) blocks wrap-up. Options: skip now (rec) / wait.
- domain-move DONE and merged (8342246). Started observability.
- QUESTION observability: 6 agent tasks done; parked at human task 8a4c8b44 (secret TCGREPLAY_PLAYER_ID_KEY before merge/push, doctl apply, alert recipients, measure memory, Insights retention, JSON parsing). Needs figures (a)-(e).
- User chose: wait for figures (observability). Child parked; send figures (a)-(e) to it when user provides them. Offered early merge for deploy if user asks.
- User: ignore measurement collection; will measure while live. Relayed to observability child: defer human tasks, keep limits, provisional retention figure marked to-confirm, finish and hand back DONE.
- observability DONE, merged (5647421). Started privacy-terms-disclaimer child.
- privacy task3 goal-check: user chose accept (criterion 5 → launch-docs launch entry). I add About panel to launch-docs plan.
- privacy DONE, merged (c32a9d7). Started visitor-analytics child.
- visitor-analytics: tasks 1-4 done; switch-on human task deferred to maintainer (consistent with user's deferral of live steps).
- visitor-analytics finished-step commit blocked by permissions; user allowed the workflow commit, relayed.
- visitor-analytics: I ran finished goto (user-approved commit; child blocked by classifier), merged b582cd3, ticked deferred task 5 heading (criteria unticked) so status=done. Started chatgpt-submission-prep.
- Resumed implement-epic run (2026-11-19): restarting child for chatgpt-submission-prep (in_progress at check_goal, task 'Tell ChatGPT who is signed in'); launch-docs blocked on it.
- chatgpt-submission-prep DONE (task 7 human deferred/accepted), merged c5a184b. Created launch-docs worktree; starting child.
- launch-docs DONE (task 4 directory switch-on deferred), merged. Epic implement complete (2026-11-19).
- Ticked deferred launch-docs task 4 heading (criteria unticked) so status=done, as with visitor-analytics.

## Post-launch (2026-11-19)
- Deployed 8e01937 (healthy, AMS3) after maintainer set TCGREPLAY_PLAYER_ID_KEY; 10:59 GitHub deploy run failure is stale (pre-key). Web Analytics on (site tcgreplay.app, manual snippet).
- Insights receives app metrics but no App Platform logs (0 results AMS2/AMS3/LON1). Maintainer chose to wait on DigitalOcean support; no forwarding for now.
- Waiting on: DO support answer (logs + retention), OpenAI plugin approval (then flip chatgptDirectory.listed + What's new entry).
- Open: set TCGREPLAY_OPENAI_CHALLENGE / TCGREPLAY_REVIEWER; reconcile .do/app.yaml with live app (region ams, service tcgreplay) before doctl apps update; set TCGREPLAY_MOVED_FROM.
- .do/app.yaml reconciled with live (ams, service tcgreplay, app-level envs; readers/tests updated; docs now console-only, maintainer never uses doctl). Uncommitted.
- Discussed: redeploys drop every in-memory game. Option: persist store.Log JSON to Spaces (debounced + flush on SIGTERM, lazy restore, overlap handling, privacy page). Maintainer deferred (2026-11-19); spec offer may be raised again later.
