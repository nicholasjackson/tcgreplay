# Submitting TCG Replay to ChatGPT

The maintainer's checklist for submitting TCG Replay to OpenAI's plugin
review. Everything in this folder stays out of the website and the
container image; nothing here may be published on the site while TCG
Replay is unlisted.

1. **Verify your identity** in the OpenAI Platform's organization
   settings. Submission needs a verified developer.
2. **Build the package**: `make plugin` writes `bin/tcgreplay-plugin.zip`
   from `listing.json`, `release-notes.md`, `assets/icon.png`, the skills in
   `skills/` (each `skills/<name>/SKILL.md`), the public address in
   `.do/app.yaml` and the notice in `internal/notice/notice.json`. OpenAI
   refuses a public submission without at least one skill, and scans every
   skill for safety (allow up to 2 hours). Bump `version` in `listing.json`
   for every new submission.
3. **Upload the ZIP** in OpenAI's plugin dashboard. Fix any validation error
   in `listing.json` (or the builder, `internal/plugin`) and rebuild.
4. **Create the reviewer account and connect the MCP server.**
   - Create the reviewer account: generate a strong random password in your
     password manager, then run `htpasswd -nBC 12 openai-reviewer` (or
     `docker run --rm -it httpd:2.4 htpasswd -nBC 12 openai-reviewer`) and
     enter it. It prints `openai-reviewer:$2y$12$…`. Keep one account per
     directory review (for example `claude-reviewer` for Claude's), every
     entry made at the same cost.
   - In the App Platform console (Settings > App-Level Environment
     Variables), set `TCGREPLAY_PASSWORD_ACCOUNTS` to the printed lines,
     comma-separated, ticking **Encrypt**. In the dashboard's MCP step, copy
     the domain-verification token and set it as `TCGREPLAY_OPENAI_CHALLENGE`
     (encrypted) too, then save (App Platform restarts the playmat). Delete
     the old single-reviewer setting if it is still there: it is no longer
     used.
   - Check the start-up line in Runtime Logs reports the password accounts
     ("2 password accounts"), and that
     https://tcgreplay.app/.well-known/openai-apps-challenge returns exactly
     the token; then verify the domain in the dashboard.
   - Connect, sign in on the TCG Replay sign-in page with the reviewer
     account, and wait for the tool scan to list the tools with their
     titles and labels.
   If the dashboard's connection is refused at registration, note the
   redirect address it used before changing the sign-in service.
5. **Review details.** In Review information > Reviewer credentials, choose
   **Username and password** and give the reviewer account: login URL
   https://tcgreplay.app, the username (`openai-reviewer`) and its password,
   with sign-in steps such as "Connect TCG Replay; on the TCG Replay sign-in
   page, enter the username and password in the form below the Google
   button". Enter them in the dashboard only, never in this repository; the
   plain password lives only in your password manager.
6. **Record the demo video** from `demo-script.md`. Add its URL as
   `review.demo_recording_url` in `listing.json` and rebuild, or enter it in
   the dashboard if the form allows.
7. **Run MCP Inspector first**: connect to https://tcgreplay.app/mcp,
   sign in through its Auth settings with the reviewer account on the TCG
   Replay sign-in page, list the tools (labels,
   titles and the profile marker on `get_account`), and call `get_account`,
   `import_log` with the example game, and `get_state`.
8. **Submit** for review.
9. **After approval**, publish the listing and tell the launch-docs owner to
   switch `chatgptDirectory.listed` on, so the website's connection docs
   point at the listing.

After publication:

- Tool names, parameters and results must stay backwards compatible: OpenAI
  holds a plugin with changed tools for re-review.
- Any change to the listing (text, icon, test cases) needs a new `version`
  in `listing.json`, a new package and a new review.
- Players already connected see changed tool descriptions only after
  refreshing the connection; say so in the release notes.
