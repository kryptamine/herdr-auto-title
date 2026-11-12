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
The connected-machine probe below used **herdr-app 0.1.15** and **hc 0.1.20**
on macOS, with two Linux cores. A Linux desktop window remains unmeasured.
The Windows checks below do not establish full Windows support.

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

- **The macOS Codex probe shared one daemon.** The first Codex's child,
  `app-server-daemon`, held the rollouts of both terminals, and the second Codex
  had no child and held none. So an open rollout says nothing about which terminal owns it, and after
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
- **Open files** are read with `lsof` on macOS and `/proc/<pid>/fd` on Linux.
  The connected-machine probe below measured directly held Codex rollouts on
  Linux; the other Linux agent routes remain unmeasured. Windows uses native
  process handle snapshots; the probe and limits are recorded below.

### Connected machines

The desktop adapter includes every machine in the app's `ls`, joining panes
to `terminal.list` by their complete address. On hc 0.1.20, that socket method
already reports remote terminals' titles, cwd and foreground processes;
`hc ls --json` is still the reduced CLI view. No `terminal.get` is needed.

Machine prefixes are part of every remote space, tab, pane and terminal id.
`box/t1` cannot match local `t1` or another machine's `other/t1`. A remote PID
never enters the local session finder, and a remote directory or session
reference never reaches a local transcript or Git reader. Renames use the
qualified tab id through the app's existing `tab.rename` command.

The core's `machine.exec` optionally enriches remote panes with a Python 3
standard-library helper. Each machine gets one batch per poll, sent as
`argv: ["python3", "-c", ...]` with base64 stdin, never a shell line. The
helper is embedded in the binary; nothing is installed remotely. A batch has
a one-second client deadline: a remote round trip exceeded 500 ms even with
session reads disabled. The server timeout is no longer than the client's
remaining deadline, and `ExecMachine` caps any caller at one second. Nonzero
or missing exit codes, `timed_out`, truncated output and invalid JSON discard
the result. An unavailable helper falls back to that remote terminal's OSC title, process
or directory; the next poll tries again without retaining stale metadata.

- **Git is read from `.git`, without running Git.** Linked worktrees, a rebase's
  branch, detached HEAD and `refs/remotes/origin/HEAD` are supported. Repository
  identities include the machine, so identical paths on two hosts differ.
  `HERDR_AUTO_TITLE_BRANCH_MAX=0` disables these reads.
- **Codex's native name needs a directly owned rollout on Linux.** Only the
  foreground process's own `/proc/<pid>/fd` is read. Exactly one rollout must
  be held, and an `app-server*` process is excluded. Multiple rollouts, a shared
  daemon or a shell foreground give no native name; cwd and modification time
  never choose one. The held path identifies its Codex home, including a custom
  home. Its latest `session_index.jsonl` entry supplies `thread_name`; when no
  entry names that session, `state_*.sqlite` supplies `threads.name` or
  `threads.title` through SQLite `mode=ro`. The index scan covers its last
  8 MiB; a name outside that window uses the same database fallback.
  No rollout prompt is read.
  `HERDR_AUTO_TITLE_TRANSCRIPT=false` disables this lookup. In session-only
  mode there is no remote opening-prompt fallback.
- **An unavailable tab stays in the snapshot.** This retains its manual-name
  lock across a disconnect and restart. With no joined pane it is not renamed;
  clearing a manual name resumes automatic naming after reconnection.

The live batch probe covered four panes on one Linux machine and five on
another. Both returned matching pane ids, two native Codex names each, and
their checked-out branches, with no timeout or truncation. The batches took
204 ms and 240 ms respectively; a later five-pane Git-only batch took 537 ms.
Native names were available on terminals with empty OSC titles. These
measurements establish the remote Linux route, not native-name lookup on a
local macOS or Windows desktop.

With the desktop process in a dedicated foreground tab, both machines' Codex
terminals with empty OSC titles showed their full native name after focusing
the agent pane. Disposable Python terminals changed their OSC title from
`Alpha remote probe` to `Beta remote probe`; a manual name stayed until cleared,
then the new automatic title returned. The probe tabs were closed afterwards.

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
implements `herdr.Client`. These things differ from TUI mode:

- **Panes and workspace tokens are off**, because they have no equivalent. The
  client does not implement `herdr.PaneRenamer` or `herdr.TopicReporter`, and
  the poll loop skips what a client does not offer, whatever the configuration.
- **Manual names live in `manual-names.json.desktop`**, so the two modes do not
  overwrite each other's file.
- **`Server()` is empty**, so a window restart is not a successor and the process
  stays. It logs failed polls until the window returns.
- **A pane the core does not list yet is left out.** `ls` and `terminal.list` are
  separate reads. Its siblings still name the tab; a tab with no joined pane
  keeps its existing label until a pane returns.
- **Connected machines are included**, using their own terminal metadata and
  optional remote reads as described above. TUI's local socket never reports
  its saved SSH machines, which run Auto Title themselves.

For local panes an agent's session arrives as it does in TUI, in
`agent_session`, so the readers are the same. The terminal title still outranks
them: a Claude Code or Grok Build tab reads the transcript only when its title
says nothing, as after `/clear`, and
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

| | macOS | Linux | Windows |
|---|---|---|---|
| Service | LaunchAgent `io.github.kryptamine.herdr-auto-title` in `~/Library/LaunchAgents` | `herdr-auto-title.service` in `$XDG_CONFIG_HOME/systemd/user` | Task Scheduler task `herdr-auto-title-<user SID>` |
| Binary | `~/Library/Application Support/herdr-auto-title/bin/` | `$XDG_DATA_HOME/herdr-auto-title/` | `%LOCALAPPDATA%\herdr-auto-title\bin\herdr-auto-title.exe` |
| Log | `~/Library/Logs/herdr-auto-title.log` | the journal | Run `desktop` in a terminal for logs |

- **The service runs a copy of its own.** Whatever ran the install, a plugin
  checkout or a download, may move or go. The copy is written beside its place
  and renamed onto it. On Unix a running copy keeps its file. Windows stages
  the copy first, disables and stops the task, then replaces the stopped image;
  Windows can take a moment to release that file after the stop returns.
  Installing from the copy itself only restarts the service on every platform.
- **`desktop status` prints `{"version": …, "running": …}`**, with `latest`
  when a newer release is out, so nothing outside the binary has to know the
  service's label, its unit or how each manager says it runs: launchd counts
  as running only with `state = running`, since a failed agent stays loaded
  while it waits out its throttle.
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
- **Windows uses the current user's interactive token**, a SID-specific login
  trigger and least privilege, without a stored password. Windows PowerShell
  5.1 calls the Task Scheduler COM API, avoiding localized `schtasks` output.
  Encoded commands preserve Unicode and quote paths as literal strings;
  no terminal text reaches the shell. Registration starts the task immediately;
  its login trigger starts it on subsequent logins. See Microsoft's
  [interactive-token semantics](https://learn.microsoft.com/en-us/windows/win32/taskschd/security-contexts-for-running-tasks).
- **Windows retries a failed poller after five seconds.** The internal
  `desktop service` entry point drops its console and owns a windowless
  `desktop` child through a kill-on-close
  [Job Object](https://learn.microsoft.com/en-us/windows/win32/procthread/job-objects).
  The job is assigned during creation, so even an immediate task stop cannot
  orphan a child. A zero exit ends the service; a nonzero exit restarts the
  poller. Stopping the task closes the job and kills the child as well.
  Battery use does not prevent or stop the task, and there is no time limit.
  This uses the existing `golang.org/x/sys` version, with no new dependency.
- **Task Scheduler's retry setting is only a startup fallback.** It is set to
  one minute and 999 attempts, but measured task actions exiting with code 7
  did not restart, whether started on demand or by a registration trigger.
  The native supervisor implements the poller's restart contract instead.
  Explicitly stopping the task or terminating its supervisor stops the whole
  service. Run `desktop install` or log in again to resume it; recovery from
  termination of the supervisor itself is not promised.
- **Windows self-uninstall retires the running image first.** Once the task is
  removed, the installed uninstaller renames itself to a unique `.old` path.
  A hidden cleanup process waits for it to exit, then deletes that exact file
  and the empty `bin` directory. A concurrent new install is not its target.
  Uninstall from another copy deletes the stopped binary synchronously.

Measured on macOS 27.2: install starts the agent, five installs in a row leave one
instance running, `kill -9` is restarted within the throttle, an instance displaced
by a newer claim stays down, and uninstall leaves the three paths gone. Linux
is not measured yet, and has to be before this ships.

Measured on Windows build 26200 on 2026-11-06 with Go 1.26.3 and Windows
PowerShell 5.1: the opt-in lifecycle test registers a uniquely named real task
that runs a harmless copy of the test executable, never the user's poller.
It exercises install, replacement while running, a nonzero worker exit, a killed
worker, a clean exit remaining stopped, install and uninstall invoked from the
installed image, and repeated uninstall. The fixture path includes spaces,
an apostrophe and Unicode. Unit tests also verify that closing the job kills its
child and cancellation releases the process. No reboot or logout is performed;
the login trigger is inspected through the registered task definition. Readback
asserts the current user's SID, interactive token, least privilege, both enabled
triggers, single-instance policy, exact action, unrestricted battery operation
and unlimited runtime. Uninstall also preserves an existing `config.env`.

The native job-list process attribute requires Windows 10 or newer. Windows
ARM64 uses the same typed API bindings; a cross-build does not establish native
ARM64 behavior. The opted-in task tests and ordinary gate are described in
[development](../development.md#tests--seconds-no-herdr-involved).

## Updates

The service tells the person when a newer release is out, and the mod's popup
installs it. Nothing updates itself.

- **Once a day, a minute after start**, and an hour after a check that failed,
  `internal/release` sends a `HEAD` to `releases/latest` and reads the version
  from the `/releases/tag/v…` it redirects to. A page, not the API: the API
  limits callers without a token, and a redirect needs no JSON. Only an
  `X.Y.Z` is taken, since it reaches the popup and `install.sh`'s URL.
  `releases/latest` names only a published release, and a release is published
  only once its binaries are attached ([installation](./installation.md)).
- **The release offered is kept in `latest-release`**, one line in the state
  directory beside the instance claims, and `desktop status` reports it as
  `latest` while it is newer than the copy. It is written before the popup
  opens, since the popup asks `desktop status` the moment it does, so a
  release already in the file is not offered again, not even at a new login.
- **The service opens the popup with `{"do":"mod.open"}`** on the app socket,
  the command `keys.json` binds. The app toggles a mod's overlay, so a popup
  already open at that moment closes instead.
- **A refusal is final, no answer is not.** Installed by hand there is no
  mod, and the app answers `no mod folder`: the service logs the release and
  keeps it. A closed window answers nothing, so the file is removed again and
  the next check, an hour later, offers it again; until then a popup opened by
  hand offers only the mod's own `VERSION`.
- **The popup offers the newer of its own `VERSION` and `latest`.** The
  download and `desktop install` are the same for any release, so a mod older
  than the release can install it; a newer mod then arrives with the gallery.
- **Only a checksum**, as for any install from the popup: it catches a broken
  download, not a compromised repository. The person still presses the button.
- **TUI never checks**, since Herdr owns its plugin's updates.

Measured on herdr-app 0.1.13: `mod.open` answers `{"ok":true}` and opens the
popup, an unknown mod answers `{"error":"no mod folder \"…\""}`, and there is
no `notify` command on the socket, so a toast is out of reach.

## The gallery mod

`mod/` is what herdr.dev/mods lists once the repository carries the
`herdr-mod` topic: `mod/mods/auto-title/`, its `RECIPE.md` and `preview.png`.
The mod is a popup that runs `desktop install`, so the binary stays the one
place that knows how a service is set up.

- **It installs the release it belongs to**, `VERSION` in `service.js`, which
  release-please bumps with `herdr-plugin.toml` and
  `TestModInstallsThisRelease` holds to the binary's version, or a newer one
  the service found ([updates](#updates)). A binary runs only after its
  checksum matched `SHA256SUMS`, as in [installation](./installation.md).
- **The install runs in a terminal tab, not through `exec`**, which gives up
  after 30 s by default, too soon for a slow download. `run("terminal.open")` returns
  nothing to follow, so `install.sh`, beside the mod, writes its result to a
  status file in a unique folder under `appHome()/data`, which the popup reads
  every second using `fs/promises`.
  Under `mods/`, a saved file would reload the mod mid-install.
- **`terminal.open` leaves a login shell in the tab** once its `argv` exits:
  measured on herdr-app 0.1.13, it runs `zsh -lic '"$@"; exec "$0" -li'` with
  `keep` set. Every terminal has its address in `HC_TERMINAL_ID`, so
  `install.sh` ends with `hc kill` on it, which closes the tab.
- **It knows only the copy's path** from `internal/autostart`. Everything else
  it asks the copy: `desktop status` for its version and whether it runs,
  `desktop install` to start it. A change to that path changes the mod too.
- **It has no uninstall button.** Removing is rare and needs no popup:
  `desktop uninstall`, run on the copy by hand, as the README shows.
- **The topic waits for a release carrying `desktop install`.** The gallery
  lists a repository within the hour, and until that release the mod names
  one whose binary answers `desktop install` with its usage line.
- **It is a popup, not a block in a layout.** Measured on herdr-app 0.1.13, a
  new mod folder with `"place": "bottom"` was docked nowhere, even after a
  restart, and the shipped `main` layout accepts no change of structure, so a
  strip would ask each person to clone `main` and edit it. A popup with
  `"place": "none"` needs no layout; `keys.json` binds `cmd-alt-t` to
  `mod.open`, as the palette lists no mod of the person's own.
- **It opens in the bottom right corner**, out of the way of the terminal it
  covers, since the service opens it unasked. `at` takes only `"top"` of the
  strings, measured on herdr-app 0.1.13: `"bottom"`, `"right"` and
  `"bottom-right"` all fell back to the pointer. A point far past the window
  is clamped into its bottom right corner, about 8px in, so `mod.json` gives
  `{ "x": 100000, "y": 100000 }`; nothing tells a mod the window's size.
  **This is a workaround**, resting on that clamping rather than on anything
  the app documents. Should a release stop clamping, the popup opens off
  screen with `mod.open` still answering `ok`, so the release counts as
  offered unseen; a corner anchor for `at` is what would replace it.
- **`herdr-app shot` returns a stale frame while the window is behind another
  app**: an open popup was missing from it. Running `herdr-app` first raises
  the window, and the frame after it shows the overlay.
- **The popup can close mid-install**, and its copy and wait loop with it.
  `install.sh` therefore deletes its own download, and the popup reads the
  copy afresh when it opens again. A closed popup leaves its small status
  directory behind; the installer still removes downloaded files.
- `herdr-app check` writes `gpui-kit.d.ts` and `jsconfig.json` into the mod's
  folder; `.gitignore` keeps them out. `window()` is null there, so the
  platform comes from `os`.
- A second window for a screenshot needs `HERDR_APP_HOME` short enough for a
  socket path, and it attaches to the same herdr-core as the first.

## Not settled

- A window that takes over 5 s answers `the window did not answer`. By the
  source, the request is still queued, so it may apply later. Not measured.
- Clicking `+` itself. The tab it makes was reproduced by calling `tab.new` with
  the same arguments.

## Windows transport probe

### Gallery mod on Windows

Measured on Windows on 2026-11-09 with herdr-app 0.1.14, hc 0.1.19 and
Windows PowerShell 5.1.26100.9444:

- A mod's `exec("powershell.exe", ["-NoProfile", "-ExecutionPolicy", "Bypass",
  "-File", script, ...args])` preserves spaces, Unicode, apostrophes, `$`, `&`
  and `;` as literal arguments. A harmless script returned code 7, its exact
  UTF-8 stdout and separate stderr. No interpolated command string is needed.
- `run("terminal.open", {argv, name})` returns `null`, with no terminal ID.
  The same arguments reached a script in a new tab; `HC_TERMINAL_ID` identified
  that tab and `hc kill` closed it without touching the original terminal.
- `install.cmd` locates Git Bash beside Git or via `git --exec-path`, validates
  its Windows runtime, and runs the shared `install.sh`. Only an unavailable
  Bash selects `install.ps1`, using `Invoke-WebRequest` and .NET SHA256;
  an install failure never retries through another route. Neither needs Go.
- A fixed PowerShell `-Command` reads `LOCALAPPDATA`; paths are never inserted
  into that command. The command also checks whether the copy exists, including outside
  the user home. Native `fs/promises` handles staging and status. The copy path follows `internal/autostart/scheduler.go`.
- Execution-policy restrictions can still block the fallback's `-File` entry.
  CMD reports that startup failure to the popup and keeps the terminal error
  visible. It does not change machine or user policy.
- CMD receives a validated version and a generated relative staging path;
  its working directory is the mod folder. This keeps `&` in a user profile
  path out of CMD's argument parser, even when that path has no spaces.
- Windows can terminate a whole terminal without running PowerShell's `finally`.
  An open popup therefore falls back to its existing ten-minute timeout and
  cleanup. Closing the popup too can leave a temporary folder after a forced
  kill; normal success and failure delete downloaded files in `finally`.

The live popup reached "Not installed" with an `Install 0.21.0` button and no
app errors. Local HTTP fixtures exercised successful installation, missing
releases, checksum mismatches, failed `desktop install`, and ARM64/AMD64 asset
selection under emulated environment variables, using a stub executable that
only recorded its arguments. No real login service was installed in this probe.
The installer CI matrix runs the same fixtures on Linux, macOS and Windows,
each on x64 and ARM64; Windows exercises both Bash and PowerShell routes.
The current `service.js` was also exercised in the live app with a harmless
CMD stub: status lookup, terminal launch, native status polling and directory
cleanup completed without changing the original terminal or installing a
service. The app reported no errors afterward.

### Socket transport

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
