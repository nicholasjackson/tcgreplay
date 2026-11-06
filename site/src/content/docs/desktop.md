---
title: Running it on your desktop
description: Install the tcgreplay playmat on your own computer and use it with the Claude desktop app.
order: 30
---

You can run the playmat on your own computer and use it with the **Claude desktop app**. There is no sign-in, no limits on players, and your games stay on your machine. It runs one game at a time.

## What you need

- **Go 1.27 or newer** and **Node.js 22 or newer**.
- **git**, to download the source.
- The **Claude desktop app**.
- Tested on **Linux and macOS**. Windows is untested.

## Install

Download the source and build it:

```sh
git clone https://github.com/nicholasjackson/tcgreplay.git
cd tcgreplay
make install
```

`make install` builds the board and puts the `tcgreplay` program in Go's install folder. To see where, run:

```sh
echo "$(go env GOPATH)/bin/tcgreplay"
```

## Connect the Claude desktop app

Open the Claude desktop app's MCP settings file, `claude_desktop_config.json` (in the app: **Settings → Developer → Edit Config**), and add tcgreplay with the path from above:

```json
{
  "mcpServers": {
    "tcgreplay": { "command": "/home/you/go/bin/tcgreplay" }
  }
}
```

Replace `/home/you/go/bin/tcgreplay` with your path. Save the file and **restart the Claude desktop app**.

**Check it works.** In a new chat, ask:

```prompt
What can the tcgreplay playmat do?
```

## The board

The board is at **http://127.0.0.1:7777**. Open it in your browser while you play: there is no sign-in on the desktop. It keeps running across app restarts.

For more about building and running it, see [the source repository](https://github.com/nicholasjackson/tcgreplay).
