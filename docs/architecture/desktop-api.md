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
socket request, unless it says otherwise. On **herdr-app 0.1.13** and **hc
0.1.18** the fields the code reads, renames, the `+` menu, Claude Code's session
and closing a space were measured again; what changed is said where it applies.
Linux remains unmeasured. The Windows checks below do not establish full
Windows support.

Desktop support is tracked in
[#188](https://github.com/kryptamine/herdr-auto-title/issues/188).

## Two sockets

| Socket | Default path | Speaks | Answers |
|--------|--------------|--------|---------|
| App | `~/.config/herdr-app/herdr-app.sock` (`$HERDR_APP_HOME`) | one JSON line each way | spaces, tabs, panes, focus, commands |
| Core | see below | JSON-RPC 2.0, one message per line | terminals: title, status, foreground process |

The core's socket on macOS and Linux is the first of these, following
herdr-core's `docs/security.md` for the `default` session; only the last was
measured, on macOS. On Windows it is a named pipe, below.

| When | Path |
|------|------|
| `XDG_RUNTIME_DIR` is set | `$XDG_RUNTIME_DIR/herdr-core/default.sock` |
| `/run/user/<uid>` exists | `/run/user/<uid>/herdr-core/default.sock` |
| `XDG_STATE_HOME` is set | `$XDG_STATE_HOME/herdr-core/default/default.sock` |
| otherwise | `~/.local/state/herdr-core/default/default.sock` |
 The app socket is not
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
Only `{"status":true}` says which pane has focus, and only for the shown tab,
so the client keeps each tab's last focused pane and hands it on as the tab's
layout, the way TUI's snapshot carries it; a tab never shown has none.
Any request the app cannot serve answers `{"error":"…"}`, and an `ls` or
`status` that does fails the poll rather than reading as an empty session.

- **There is no `named` in `ls`.** Mods get one from `spaces()`; a client on the
  socket recognizes an unnamed tab by its name equalling its position, the same
  ambiguity as in TUI. A tab renamed to its own position reads as unnamed.
- **`status` in `ls` is the one to read.** It is the app's own word, `done` while
  the user has not looked, where hc 0.1.14's `terminal.list` still said `idle`.
  A Pi report of `error` read as `blocked`. In hc 0.1.18 `terminal.list` carries
  `status` as an object, `{"app":…,"state":…}`, not a string.
- **A tab started from the `+` menu is named.** `new-menu` passes
  `name: agent.name` to `tab.new`, so it reads as a user's tab called "Claude
  Code" and manual rename protection would never touch it. A tab started by
  `tab.new` with no name is unnamed. Both were read back from `ls`. The client
  lists those names by hand, so one the app adds later locks its tab until listed.
  The app's own list is in its binary, each entry a `"name"` beside a `"run"`;
  0.1.12 and 0.1.13 added PiG and DeepSeek Harness.
- **A launcher's tab is seen before its agent.** For about a second after
  `tab.new`, `ls` gives the terminal no `app` and `terminal.list` no
  `foreground`, and a poll in that window would lock the launcher's name as the
  user's. `argv` is there from the start: `["fish", "-l"]` for a plain tab, the
  wrapper below around the agent for one the launcher started, so a launcher's
  name on a terminal that starts a program counts as unnamed too.

## What the core reports

- **All five agents report OSC 7501 with nothing installed.** The `app` values:
  `claude-code`, `codex`, `pi`, `grok-build`, `antigravity`.
- **`foreground` is `null` for the first moments of a terminal.**
- **`foreground` is not the agent in a tab started with `argv`.** `tab.new`
  wraps it, as `fish -lic "command $argv[2..]; exec $argv[1] -li"` in 0.1.13,
  and the agent runs as that shell's child in the shell's process group, so
  `foreground` stays `fish`. Typed at a prompt, the agent is the foreground
  process itself.
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
- **Open files** are read with `lsof` on macOS and `/proc/<pid>/fd` on Linux,
  the latter unmeasured on a live agent. Windows uses native process handle
  snapshots; the probe and limits are recorded below.

### What reading them costs

`ps -axo pid=,ppid=` takes about 33 ms with 860 processes, and `lsof -p` about
55 ms. A snapshot that read every session of five agents, three of which need
`lsof`, took 140 to 155 ms, against 2.5 to 3 ms for one that reused them.

So a read is reused for `sessionRefresh`, 5 s, unless the pane's agent, `pid`,
foreground or `turn` moves first. The time limit is what follows a new session:

- **`/clear` moves none of those.** `turn` stayed where it was.
- **`turn` counts only what `terminal.send` delivered.** A prompt typed at the
  keyboard leaves it alone: a Claude Code tab in real use read `turn` 0.

A failed `ps` or `lsof` keeps the session the pane had, since it says nothing
about that session, unless the pane's agent or `pid` has changed since.

A session with no file under `conversations/` counts as Antigravity's newest,
since every older one has had its first prompt. Whether `/new` makes that file
at once was not measured, and neither was the `pid` file a crashed Claude Code
leaves behind: a shallower process that reused its `pid` would be read first.

## Closing a space

`space.close` refuses to run without `"confirmed": true`, since it ends the
space's terminals. In 0.1.9 it answered before they had exited, and for about
three seconds the app adopted those still running into a new space named after
their directory. 0.1.10 fixed that: polled every 50 ms for five seconds after the
close, no such space appeared in 0.1.13. `recent` gains the closed space's name.

## With no window open

After `app.quit`, `herdr-app.sock` stays on disk and refuses the connection, so
its existence says nothing about a window. The core keeps answering, terminals
keep running, and a rename made before the quit is still there when the window
reopens.

## The desktop mode

`herdr-auto-title desktop` is the same poll loop behind `internal/desktop`, which
implements `herdr.Client`. Four things differ from TUI mode:

- **Panes and workspace tokens are off**, because they have no equivalent. The
  client does not implement `herdr.PaneRenamer` or `herdr.TopicReporter`, and
  the poll loop skips what a client does not offer, whatever the configuration.
- **Manual names live in `manual-names.json.desktop`**, so the two modes do not
  overwrite each other's file.
- **`Server()` is empty**, so a window restart is not a successor and the process
  stays. It logs failed polls until the window returns.
- **A pane the core does not list yet is left out.** `ls` and `terminal.list` are
  separate reads. Its siblings still name the tab, and a tab with no other pane
  gets the fallback name for that poll.

An agent's session arrives as it does in TUI, in `agent_session`, so the readers
are the same. The terminal title still outranks them: a Claude Code or Grok Build
tab reads the transcript only when its title says nothing, as after `/clear`, and
an Antigravity tab, whose title is empty, always does.

`HERDR_AUTO_TITLE_ICONS` needs a Nerd Font in the app too, and its own setting:
the tab bar is a mod, drawn in `ui_font`, not in the terminals' `font`. Herdr's
default, Geist, has no such glyphs, so set `ui_font` in the app's `settings.json`
to a Nerd Font, such as `JetBrainsMono Nerd Font Propo`. That changes every mod.

A tab already named when the process first starts is renamed: the first poll
claims nothing, as in TUI, since nothing there can tell a user's name from an
earlier run's. A rename after that is the user's, and stays.

Launcher names are defaults only while the first observed label is unchanged.
After it changes, returning to `Codex` or `Claude Code` is a manual rename.
The adapter keeps the raw label and marks defaults separately, so a saved
manual-name lock still matches after restarting the client.

## Starting at login

The app has no plugin system, so nothing starts Auto Title with it. Instead
`herdr-auto-title desktop install` has the OS start the desktop mode at login,
and `desktop uninstall` takes that away. Both live in `internal/autostart`.

| | macOS | Linux |
|---|---|---|
| Service | LaunchAgent `io.github.kryptamine.herdr-auto-title` in `~/Library/LaunchAgents` | `herdr-auto-title.service` in `$XDG_CONFIG_HOME/systemd/user` |
| Binary | `~/Library/Application Support/herdr-auto-title/bin/` | `$XDG_DATA_HOME/herdr-auto-title/` |
| Log | `~/Library/Logs/herdr-auto-title.log` | the journal |

- **The service runs a copy of its own.** Whatever ran the install, a plugin
  checkout or a download, may move or go. The copy is written beside its place
  and renamed onto it, so a running copy keeps its file and an install run
  from the copy itself does not truncate it.
- **`desktop status` prints `{"version": …, "running": …}`**, so nothing outside
  the binary has to know the service's label, its unit or how each manager
  says it runs: launchd counts as running only with `state = running`, since a
  failed agent stays loaded while it waits out its throttle.
- **Install again to upgrade.** It replaces the copy and restarts the service:
  launchd unloads and loads the agent again, since `bootstrap` refuses a label
  already loaded, and asks again while the old one is still leaving; systemd
  is told `restart`, not `start`.
- **Only a failure is restarted.** `KeepAlive` is `SuccessfulExit: false` and
  `Restart=on-failure`. A clean exit is a newer instance claiming the session,
  as `make run-desktop` does, and restarting the old one would take it back.
- **A closed window or an app update needs nothing.** The process outlives the
  window and logs failed polls, ever more rarely, until it answers again.
- **Uninstall leaves `config.env` and the manual names.** It stops the service
  and removes its definition, the copy and the macOS log. What is already gone
  is not an error.
- **The service does not see the login shell's environment.** Settings come
  from `config.env`, as in TUI; a `CLAUDE_CONFIG_DIR` or `CODEX_HOME` exported
  in a shell profile reaches neither launchd nor the systemd user manager.
- **Windows is not supported yet**, and says so.

Measured on macOS 27.2: install starts the agent, five installs in a row leave one
instance running, `kill -9` is restarted within the throttle, an instance displaced
by a newer claim stays down, and uninstall leaves the three paths gone. Linux
is not measured yet, and has to be before this ships.

## Not settled

- A window that takes over 5 s answers `the window did not answer`. By the
  source, the request is still queued, so it may apply later. Not measured.
- Clicking `+` itself. The tab it makes was reproduced by calling `tab.new` with
  the same arguments.

## Windows transport probe

Measured on Windows on 2026-11-05 with herdr-app 0.1.13 and hc 0.1.18.
Windows uses the same client and poll loop, with platform-specific endpoint
discovery, transport and process enumeration. No additional dependency is needed.

- The app accepts Go's Unix socket connection at
  `C:\Users\Admin\.config\herdr-app\herdr-app.sock`. Both `ls` and `status`
  decoded successfully.
- The core answers newline-delimited JSON-RPC over a Windows named pipe. The
  current terminal supplied `HC_TERMINAL_SOCKET` as
  `\\.\pipe\herdr-core-Admin-default.sock`; opening that path directly and
  requesting `terminal.list` worked. Discovery uses the default session and
  `USERNAME` stripped to ASCII letters and digits, matching
  [hc 0.1.18](https://github.com/herdrdev/herdr-core/blob/v0.1.18/crates/herdr-core/src/platform/windows.rs).
  The live client worked without using `HC_TERMINAL_SOCKET`.
- The app reported `app: "codex"`. The core reported `pwsh.exe` as the
  foreground process, with its current repository directory; the launch
  directory remained the user's home. The existing resolver produced a
  repository, agent and topic title from the live terminal title, with its
  braille spinner removed. No transcript lookup was needed.
- On a new test tab, a Unicode title round-tripped unchanged,
  `"  trimmed probe  "` read back as `"trimmed probe"`, and an empty name
  restored position `"2"`. A nonexistent tab mapped to `CodeTabNotFound`.
- The test tab was closed afterward. Independent `ls` and `status` readback
  confirmed the original tab name and focus, no extra tab, and no app errors.
- The sandbox could not access the core pipe; the same read succeeded in the
  current user's environment. No server restart or permission change was needed.
- The production poll loop ran for three seconds against a temporary terminal
  emitting a Codex status and title. It generated `codex` plus the topic, then
  preserved a manual `Codex` label across three polls and a fresh client and
  loop loading the saved lock. A test wrapper restricted writes to that tab.
- Native process enumeration found the test process under its actual parent.
  Fixture tests cover Claude's per-pid session files. Pipe tests cover a real
  request/reply and cancellation while a server withholds its reply.

Claude uses the native Windows process tree and the same per-pid session files.
Grok and Antigravity use the native open-file reader below. Real Claude session
transitions, alternate homes/sessions, long-running stability and app restart
recovery remain unmeasured.

The Python probe uses `herdr-app ls/status` and `hc ls/get` on Windows: Python's
Windows sockets do not expose `AF_UNIX`. CLI reads have a six-second timeout,
and stdout is UTF-8 so spinner frames and non-ASCII topics can be printed.

### Windows open files

`NtQueryInformationProcess(ProcessHandleInformation)` takes a per-process handle
snapshot. The [native layout](https://github.com/winsiderss/phnt/blob/master/ntpsapi.h)
has two pointer-sized header values, then entries containing a handle, two
pointer-sized counts and four 32-bit values. Only the handle is consumed.
Buffers grow up to 16 MiB and the returned count is checked before indexing.
The native entry point is resolved at runtime; failure preserves the previous
session for the same agent process rather than claiming it holds no session.

Each handle is duplicated into this process with `DuplicateHandle`, inspected
with `GetFileType`, and closed here. The source handle is never closed. Only
disk files and directories reach `GetFinalPathNameByHandleW`; pipes and other
objects are skipped. `FILE_NAME_OPENED` avoids normalized-path traversal and
the returned DOS/UNC prefix is removed before session matching. Matching also
accepts Windows separators. A process that exited, a closed handle and an object
type that cannot be duplicated are skipped; access failures remain errors.

Cancellation is checked between native calls. A filesystem call already in
progress cannot be interrupted by that context; slow network filesystems remain
unmeasured. No external utility, debug privilege or extra dependency is needed.

On 2026-11-05, Grok Build 1.0.70 held its actual `events.jsonl` open on Windows.
Two Grok processes in the same directory resolved to their separate session IDs.
The native read of the first process tree took about 30 ms under the race
detector. The production poll loop named the second test tab
`grok › Second Windows session probe`, using the actual session summary.
After `/new` and another no-tools prompt, that tab resolved the new session ID
and became `grok › New Windows Session Probe Instruction`; the first process
still resolved its original ID. Both test tabs were closed after the probe.

This machine's Grok did not report an app identity by itself. The second test
launcher emitted an explicit OSC 7501 status before starting Grok. Core 0.1.18
normalized `app=grok-build` to `grok` in both core and app reads, so both names
route to the same reader. Without integration status, session routing is not
attempted. The status emission was test-only; no integration was installed.

Regression tests hold real files and directories with spaces and non-ASCII
paths, exercise both session matchers, reject truncated handle snapshots, and
verify twenty reads leave only the fixture's original file handle. Antigravity CLI is
not installed here: its directory matching is fixture-tested, not measured with
a real agent.
