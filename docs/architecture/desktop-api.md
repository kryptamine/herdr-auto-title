---
type: doc
title: 'Herdr Desktop API'
description: 'The measured facts about the Herdr desktop app and herdr-core that a desktop mode of Auto Title rests on: the two sockets, what each reports, how renames behave, and how each agent session can be found.'
tags: [architecture, reference]
created: 2026-10-30
generated: { by: claude-code/opus-5, at: 2026-10-30T13:50:00+02:00 }
---

# Herdr Desktop API

The desktop app is a separate program from Herdr TUI and shares no agents with
it. Every fact below was measured against a live **herdr-app 0.1.9** and **hc
0.1.14, protocol generation 1** on macOS, with `make probe-desktop` or a direct
socket request, unless it says otherwise. Nothing here has been measured on
Linux or Windows.

Desktop support is tracked in
[#188](https://github.com/kryptamine/herdr-auto-title/issues/188).

## Two sockets

| Socket | Default path | Speaks | Answers |
|--------|--------------|--------|---------|
| App | `~/.config/herdr-app/herdr-app.sock` (`$HERDR_APP_HOME`) | one JSON line each way | spaces, tabs, panes, focus, commands |
| Core | `~/.local/state/herdr-core/default/default.sock` | JSON-RPC 2.0, one message per line | terminals: title, status, foreground process |

The core's path follows `XDG_RUNTIME_DIR` and `XDG_STATE_HOME` when they are
set, and `<session>` is `default` unless chosen otherwise. The app socket is not
a documented API; the core's is, by OpenRPC (`hc schema`).

Neither is named by `HERDR_SOCKET_PATH`, which belongs to the TUI server. Both
answer a process outside the app, including one in a Herdr TUI pane.

`hc ls --json` is not `terminal.list`: it is a reduced view with no `title`,
`status` or `foreground`. Read the socket.

### What one poll costs

| Call | Mean |
|------|------|
| `{"ls":true}` | 0.17 ms |
| `{"status":true}` | 0.08 ms |
| `terminal.list`, 8 terminals | 1.62 ms |
| `{"do":"tab.rename",...}` | 0.3 ms |

Each call dialed its own connection. Polling at the TUI's 500 ms costs nothing
worth optimizing, which is why the desktop mode keeps polling.

## What the app reports

`{"ls":true}` gives spaces, their tabs and each tab's terminals. A terminal
entry carries `terminal`, `pane`, `status`, `app`, `name` and `sub_agents`, and
no title, directory or process, so it is joined to `terminal.list` by address.
Only `{"status":true}` says which pane has focus, and only for the shown tab.

- **There is no `named` in `ls`.** Mods get one from `spaces()`; a client on the
  socket recognizes an unnamed tab by its name equalling its position, the same
  ambiguity as in TUI. A tab renamed to its own position reads as unnamed.
- **`status` in `ls` is the one to read.** It is the app's own word, `done` while
  the user has not looked, where `terminal.list` still says `idle`. A Pi report
  of `error` read as `blocked`.
- **A tab started from the `+` menu is named.** `new-menu` passes
  `name: agent.name` to `tab.new`, so it reads as a user's tab called "Claude
  Code" and manual rename protection would never touch it. A tab started by
  `tab.new` with no name is unnamed. Both were read back from `ls`. The client
  lists those names by hand, so one the app adds later locks its tab until listed.

## What the core reports

- **All five agents report OSC 7501 with nothing installed.** The `app` values:
  `claude-code`, `codex`, `pi`, `grok-build`, `antigravity`.
- **`foreground` is `null` for the first moments of a terminal.**
- **`foreground` is not the agent in a tab started with `argv`.** `tab.new`
  wraps it, as `fish -ic "command $argv[2..]; exec $argv[1] -i"`, and the agent
  runs as that shell's child in the shell's process group, so `foreground`
  stays `fish`. Typed at a prompt, the agent is the foreground process itself.
- **`foreground.name` can be a version.** Claude Code's reads `2.1.314`. Its
  `argv` reads `["claude"]`.
- **`title` is raw.** There is no stripped field. Spinners arrive on every frame:

  ```
  ⠋ herdr-auto-title
  ⠇ ⠇ | herdr-auto-title
  ⠹ - Waiting for response… - grok
  ```

  So this needs what [#182](https://github.com/kryptamine/herdr-auto-title/issues/182)
  asks for before a title can be compared between polls.
- **The topic is often in `title` already.** After one prompt:

  | Agent | `title` |
  |-------|---------|
  | Claude Code | a short topic, here in Russian (`Claude Code` again after `/clear`) |
  | Codex | `Reply with ok \| herdr-auto-title` |
  | Grok Build | `Request single-word ok reply without too… - grok` |
  | Pi | `π - herdr-auto-title`: no topic |
  | Antigravity | empty: no topic |

## Renaming a tab

`{"do":"tab.rename","args":{"id":...,"name":...}}` answers `{"ok":true}` and
`ls` shows the new name on the next read.

- A name is trimmed, so `"  spaced  "` reads back as `"spaced"`.
- A newline is stored verbatim, as in TUI.
- `""` gives the tab its position back.
- An unknown id answers `{"error":"no tab \"…\""}`, the only spelling of "gone".
- An unknown argument and a non-string `name` are errors. So is
  `herdr-app do tab.rename --name 123`, because the CLI parses a value as JSON
  first. A client of the CLI must pass one JSON object instead.

By the app's own guide, tabs are kept in the core's layout document, where the
newest accepted write wins. Surviving a window restart was not measured.

## Finding an agent's session

Nothing reports a session id. OSC 7501 has no field for one, and a core terminal
sets no `HERDR_*` variable, which Pi's Herdr integration requires; the other
agents' hooks were not read. So each agent has its own route, starting from the
terminal's `pid` and walking its descendants.

| Agent | Route | Two sessions in one directory | After a new session |
|-------|-------|-------------------------------|---------------------|
| Claude Code | `~/.claude/sessions/<pid>.json` holds `sessionId` | Told apart: one file per pid | `/clear` rewrites the file |
| Grok Build | its process holds `~/.grok/sessions/<dir>/<id>/events.jsonl` open | Told apart | `/new` holds both; the newest is current |
| Antigravity | its process holds `~/.gemini/antigravity-cli/brain/<id>/` open | Told apart | `/new` holds both; the newest is current |
| Codex | **none** | **Not told apart** | a rollout appears at the first prompt |
| Pi | **none**: only the session folder of its directory | **Not told apart** | a file appears at the first prompt |

- **Codex shares one daemon.** The first Codex's child, `app-server-daemon`, held
  the rollouts of both terminals, and the second Codex had no child and held
  none. So an open rollout says nothing about which terminal owns it, and after
  `/new` the old one stays open too. The terminal title carries the topic.
- **`/new` in Codex opens a dialog** asking where the conversation should run, so
  text sent straight after it is taken by the dialog.
- **Pi holds no session file open.** Its first file is stamped within about a
  second of the process starting, which tells apart sessions started at
  different times, but two started in the same second are not, and a file made
  by `/new` has no such link. Only Pi's extension API knows for certain.
- **Which held session is current** was judged by newest: by the time-ordered id
  for Grok Build and by the file under `conversations/` for Antigravity, whose
  ids are random. Returning to an older session was not measured.
- **Open files** were read with `lsof`. Linux should offer the same through
  `/proc/<pid>/fd`; Windows has no equivalent here, which leaves it Claude Code
  alone.

## Closing a space

`space.close` answers before its terminals have exited. For about the next three
seconds, those still running carry `herdr.space` labels the app no longer knows,
so it adopts them into a new space named after their directory, which then
disappears as they exit. A poll in that window sees tabs that are already going.
The closed space's `recent` list also gained an entry with an empty name.

## With no window open

After `app.quit`, `herdr-app.sock` stays on disk and refuses the connection, so
its existence says nothing about a window. The core keeps answering, terminals
keep running, and a rename made before the quit is still there when the window
reopens.

## The desktop mode

`herdr-auto-title desktop` is the same poll loop behind `internal/desktop`, which
implements `herdr.Client`. Three things differ from TUI mode:

- **Panes and workspace tokens are off**, because they have no equivalent.
- **Manual names live in `manual-names.json.desktop`**, so the two modes do not
  overwrite each other's file.
- **`Server()` is empty**, so a window restart is not a successor and the process
  stays. It logs failed polls until the window returns.
- **A pane the core does not list yet is left out.** `ls` and `terminal.list` are
  separate reads. Its siblings still name the tab, and a tab with no other pane
  gets the fallback name for that poll.

`HERDR_AUTO_TITLE_ICONS` needs a Nerd Font in the app too, and its own setting:
the tab bar is a mod, drawn in `ui_font`, not in the terminals' `font`. Herdr's
default, Geist, has no such glyphs, so set `ui_font` in the app's `settings.json`
to a Nerd Font, such as `JetBrainsMono Nerd Font Propo`. That changes every mod.

A tab already named when the process first starts is renamed: the first poll
claims nothing, as in TUI, since nothing there can tell a user's name from an
earlier run's. A rename after that is the user's, and stays.

## Not settled

- The app socket on Windows. By the source, it is an `AF_UNIX` socket at the same
  path, through `uds_windows`, and not a named pipe. Not measured.
- A window that takes over 5 s answers `the window did not answer`. By the
  source, the request is still queued, so it may apply later. Not measured.
- Clicking `+` itself. The tab it makes was reproduced by calling `tab.new` with
  the same arguments.
