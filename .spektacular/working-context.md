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
