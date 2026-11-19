---
tags: [hosting, digitalocean, deploy]
---

# Pushing to main does not apply `.do/app.yaml`

`.github/workflows/deploy.yml` asks App Platform for a new deployment (`doctl apps create-deployment`, run by GitHub Actions), which rebuilds the code against the **app spec DigitalOcean has stored**. Nothing in a push applies `.do/app.yaml`. The maintainer does not use the `doctl` CLI: the stored spec is changed in the control panel, at the app's **Settings > App Spec > Edit**, and environment values at **Settings > App-Level Environment Variables**. Never suggest `doctl apps update` or `doctl apps spec get` as the way to apply or read the spec.

- **The file records the live app; the console holds it.** On 2026-11-19 the file was brought in line with the stored spec: region `ams` (AMS3), one service named `tcgreplay`, settings at app level (top-level `envs`, which the site build and its tests read), app-wide `DEPLOYMENT_FAILED` and `DOMAIN_FAILED` alerts, domains `tcgreplay.app` (PRIMARY) and `tcgreplay.demo.gs` (ALIAS). Settings declared in the file but not yet added in the console (then: `TCGREPLAY_MOVED_FROM`, the restart/CPU/memory alert rules) do nothing until someone adds them there.
- **Apply a change by editing only the changed entries in the console editor.** Never paste the whole file over the stored spec: the file carries no secret values, so pasting it wipes every encrypted value. The editor shows encrypted values as `EV[…]` ciphertext; leave those lines as they are.
- **A new required setting breaks the next push.** Hosted mode refuses to start without some settings (for example `TCGREPLAY_PLAYER_ID_KEY`). Declaring one in the file does not set it, so a push of code that needs it fails with "container exited with a non-zero exit code" and App Platform rolls back automatically. Set the value in the console (encrypted) **before** pushing the code that needs it.
- **Saving settings in the console restarts the playmat** (and drops every in-memory game), the same as a deploy.
- The app's start-up line in Runtime Logs lists which optional settings are on, so it is the quickest check after setting values (for example "player IDs on", "no moved hosts", "visitor analytics: off").
