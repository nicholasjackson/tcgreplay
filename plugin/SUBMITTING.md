# Submitting TCG Replay to ChatGPT

The maintainer's checklist for submitting TCG Replay to OpenAI's plugin
review. Everything in this folder stays out of the website and the
container image; nothing here may be published on the site while TCG
Replay is unlisted.

1. **Verify your identity** in the OpenAI Platform's organization
   settings. Submission needs a verified developer.
2. **Build the package**: `make plugin` writes `bin/tcgreplay-plugin.zip`
   from `listing.json`, `release-notes.md`, `assets/icon.png`, the public
   address in `.do/app.yaml` and the notice in `internal/notice/notice.json`.
   Bump `version` in `listing.json` for every new submission.
3. **Upload the ZIP** in OpenAI's plugin dashboard. Fix any validation error
   in `listing.json` (or the builder, `internal/plugin`) and rebuild.
4. **Connect the MCP server.** In the dashboard's MCP step:
   - copy the domain-verification token, set it as `TCGREPLAY_OPENAI_CHALLENGE`
     in the App Platform console (encrypted), with the reviewer's email as
     `TCGREPLAY_REVIEWER`, apply `.do/app.yaml` by hand with
     `doctl apps update <app-id> --spec .do/app.yaml`, and redeploy;
   - check https://tcgreplay.app/.well-known/openai-apps-challenge returns
     exactly the token, then verify the domain in the dashboard;
   - connect, sign in with Google, and wait for the tool scan to list the
     tools with their titles and labels.
   If the dashboard's connection is refused at registration, note the
   redirect address it used before changing the sign-in service.
5. **Review details.** Give the reviewer's Google account and password, and
   the login page https://tcgreplay.app, in the dashboard only, never in
   this repository. The account must have no two-step verification.
6. **Record the demo video** from `demo-script.md`. Add its URL as
   `review.demo_recording_url` in `listing.json` and rebuild, or enter it in
   the dashboard if the form allows.
7. **Run MCP Inspector first**: connect to https://tcgreplay.app/mcp,
   complete Google sign-in through its Auth settings, list the tools (labels,
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
