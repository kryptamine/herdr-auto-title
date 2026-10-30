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
  `tab.new` with no name is unnamed.

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

| Agent | Route | Follows a new session | Rests on |
|-------|-------|-----------------------|----------|
| Claude Code | `~/.claude/sessions/<pid>.json` holds `sessionId` | Yes, `/clear` measured | a file named by pid |
| Grok Build | its process holds `~/.grok/sessions/<dir>/<id>/events.jsonl` open | Not measured | open files |
| Antigravity | its process holds `~/.gemini/antigravity-cli/brain/<id>/` open | Not measured | open files |
| Codex | a descendant holds `rollout-*-<id>.jsonl` open | **No, see below** | open files |
| Pi | none: the newest file under `~/.pi/agent/sessions/<cwd slug>/` | Not measured | a guess |

- **Codex after `/new`** kept the old rollout open, so this route can name a
  session that has ended. The terminal title already carries the topic.
- **Pi** holds no file open between writes, so two Pi sessions in one directory
  cannot be told apart. Its only reliable source is its own extension API.
- **Open files** were read with `lsof`. Linux should offer the same through
  `/proc/<pid>/fd`; Windows has no equivalent here, which leaves it Claude Code
  alone.

## Not settled

- What each socket does with no window open. By the source, `herdr-app.sock`
  refuses the connection and the core keeps answering. Not measured.
- The app socket's transport on Windows.
- A window that takes over 5 s answers `the window did not answer`. By the
  source, the request is still queued, so it may apply later. Not measured.
- Grok Build and Antigravity after a new session, and two Codex sessions in one
  directory.
