---
type: doc
title: 'Herdr Socket API'
description: 'The wire protocol Auto Title speaks, the methods it uses, and the measured facts about Herdr 0.8.2 that the rest of the architecture rests on — including the ones the originating specification got wrong.'
tags: [architecture, reference]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Herdr Socket API

Everything Auto Title knows about a session arrives over one Unix socket. The
originating specification is wrong on several protocol details, so every fact
below was verified against a live **Herdr 0.8.2, protocol 20** install with
`scripts/probe.py` or a direct socket request. **Probe before assuming anything
not listed here**, and record what a probe teaches you in this file, which is
the record. `CLAUDE.md` carries the short list of facts that would otherwise
mislead the code in silence; a new one goes there too.

## Transport

Newline-delimited JSON over the socket named by `HERDR_SOCKET_PATH`
(`internal/herdr/client.go`). A request is `{"id","method","params"}` — `params`
is required even when empty, which is why `emptyParams` exists in
`internal/herdr/protocol.go` — and a reply is `{"id","result"}` or
`{"id","error"}`.

**One request per connection.** Herdr closes the connection after answering, so
`SocketClient.Call` dials its own each time. That is not a workaround: it is why
there is no connection to lose and no reconnect logic anywhere in the plugin.
See [the poll loop](./poll-loop.md).

A malformed request is answered with an uncorrelated error frame, and the
connection is then closed.

## What Herdr gives a plugin process

A plugin the server starts inherits **the server's** environment, not the shell
of whoever installed it — which is why settings reach Auto Title through a file;
see [configuration](./configuration.md).

Alongside `HERDR_SOCKET_PATH` and `HERDR_BIN_PATH`, the 0.8.2 binary names
`HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR` and `HERDR_PLUGIN_STATE_DIR` for
a plugin process (and `HERDR_PLUGIN_ID`, `HERDR_PLUGIN_ENTRYPOINT_ID`,
`HERDR_PLUGIN_CONTEXT_JSON` with it). `herdr plugin config-dir <plugin id>`
prints the config directory, `herdr plugin list` repeats it, and Herdr creates
it empty at install time: `~/.config/herdr/plugins/config/herdr.auto-title/`
exists here.

> **Not observed live.** The variable names come from strings in the `herdr`
> 0.8.2 binary, not from the environment of a running plugin — seeing that needs
> `herdr server stop`, which closes the session. The directory and the two CLI
> commands were confirmed directly. Auto Title depends on none of it.

## The methods Auto Title uses

Three, and no others (`internal/herdr/session.go`):

- **`session.snapshot`** returns the whole session — every tab with its label,
  every pane with its directory, terminal title, agent and agent status.
  Measured at 0.47 ms and 6 KB for six panes.
- **`pane.process_info`** returns what is running in one pane. Measured at
  0.11 ms — less than the snapshot itself, because it reads the process table
  rather than serializing the session. It is one request *per pane* though, so
  a poll that asks about every pane pays for the cheapness several times over:
  on an eight-pane session the reads measured 0.17 ms each against a 1.35 ms
  snapshot, which is as much again as the snapshot they follow. Its
  `foreground_processes` holds the pane's foreground process *and that
  process's descendants*, each with `name` and a nullable `argv`. **A pane's
  revision does not track that list**: over ten minutes of a live eight-pane
  session the processes changed nine times and the revision moved with them
  four, one pane going `env` → `node` → `esbuild` → `fish` with its revision
  held at 10. A revision reports that the pane drew, nothing more.
- **`tab.rename`** takes `{tab_id, label}`. Measured at 0.16 ms median and
  0.21 ms at p95 over forty calls, against 0.99 ms for the `session.snapshot`
  preceding them. Renaming is not what limits anything.

A label is **one line**. `tab.rename` accepts a newline and stores it verbatim,
with no error and no stripping, but the tab bar renders a single line and Herdr
exposes no setting for its height. Anything a title has to say fits on one row
or does not get said.

`tab.get` and `pane.get` read one object each, and `pane.list` filters by
workspace only, never by tab. None of them is needed while the snapshot is one
call.

## Why the event stream is not used

Herdr does expose an event stream, and Auto Title deliberately ignores it.

**On subscribe, Herdr replays a backlog before delivering anything live**:
roughly the last 95 revisions of every pane, paced at about ten a second, so
around ten seconds of history for each active pane, closed panes included. Live
events queue behind that — a change made two seconds after subscribing was
observed arriving thirteen seconds later.

There is no way to skip it. `events.subscribe` takes only a subscription list,
event envelopes carry no timestamp or sequence number, and no method exposes a
stream position. A subscriber therefore spends its first seconds reacting to a
session that no longer exists, while a snapshot always describes the present.

**Do not reintroduce a subscription** without measuring again and recording the
result here.

Two further traps, if anyone does: subscription types use dot notation
(`pane.updated`) while the events they deliver arrive with snake_case kinds
(`pane_updated`), wrapped as `{"event": ..., "data": ...}`; and
`pane.output_changed` is a real event kind but is **not** an accepted
subscription type. `pane.agent_status_changed`, `pane.scroll_changed` and
`pane.output_matched` are per-pane and rejected without a `pane_id`, while
`pane.agent_detected` is global. `pane_closed` and `pane_agent_detected` carry
only pane identifiers — neither names the tab.

## What the objects carry

The wire types in `internal/herdr/session.go` mirror only the fields the code
reads, so this section describes Herdr rather than those types.

- **Pane revisions are monotonic per pane.** That is how one poll tells which
  panes moved since the last, and it is the whole basis of
  `internal/state/changes.go`.
- **`PaneInfo.cwd` is the pane's own shell, not what the user is typing into.**
  A subshell moves `foreground_cwd` and leaves `cwd` behind: probed with
  `chezmoi cd`, which runs `$SHELL` in the source directory, the pane reported
  `cwd: ~/Work/global-sso` and `foreground_cwd: ~/.local/share/chezmoi` for as
  long as that subshell lived, while `pane.process_info` listed the subshell
  alone. Both fields are null when Herdr cannot read one, and neither is the
  pane's directory on its own — see the two facts below and
  [title resolution](./title-resolution.md).
- **`foreground_cwd` is the deepest descendant's, not the foreground process's
  own.** Probed with a pane in `~/Library/Application Support/herdr-auto-title`
  running `python3` that had spawned `sleep` in `/tmp`: `cwd` reported the
  pane's directory, `foreground_cwd` reported `/private/tmp`, and
  `pane.process_info` listed the child first and its parent second. Anything a
  program starts elsewhere takes the pane's directory with it.
- **A pane's directory is the `cwd` of its own foreground process**, which only
  `pane.process_info` reports. Probed across the four panes of a live session:
  in every one the last entry of `foreground_processes` was the process whose
  `pid` equals `foreground_process_group_id`, and its `cwd` was the directory
  the pane was working in. One pane disagreed with both snapshot fields at once
  — `cwd: ~/Work/herdr-auto-title` (the shell it was started from) against
  `foreground_cwd: ~/Work/self-care-portal` (an MCP server), with the agent
  itself in `~/Work/self-care-portal`. Auto Title reads it for the pane that
  names the tab and keeps the snapshot's pair behind it — see
  [title resolution](./title-resolution.md).
- **`foreground_processes` is the pane's foreground process group.** Every
  process it listed shared the group id `pane.process_info` reports as
  `foreground_process_group_id` and the pane's controlling terminal; a
  descendant started in a group of its own without a terminal — which is how
  Claude Code runs the commands it is asked to run — was absent from the list
  while it ran. An agent's MCP servers are in it.
- **`pane.process_info` reports more per process than a name.** Each entry
  carries `pid`, `argv0`, `cmdline` and `cwd` beside `name` and `argv`, and the
  pane's entry carries `shell_pid` and `foreground_process_group_id`. Auto
  Title reads the name, the arguments and the directory; the rest is listed
  here so a future change need not probe again.
- **`PaneInfo` carries no foreground process name.** Only `pane.process_info`
  answers that, and nothing announces that a command started.
- **`PaneInfo.title` is the agent's own title**, not the terminal's. Herdr left
  it null for every Claude Code pane observed; that agent reports its topic
  through `terminal_title_stripped` instead. This is why most agent context
  reaches a title one rung below the agent source — see
  [title resolution](./title-resolution.md).
- **`PaneInfo.agent_session` says which conversation the pane's agent holds**,
  and it is null until an agent's integration reports one.
  `herdr integration install <agent>` installs the hook that does — Herdr ships
  one for seventeen agents, Claude Code among them, and `herdr integration
  status` lists them with the path each is written to. The Claude hook runs on
  `SessionStart` and calls `pane.report_agent_session` with the session id and
  the transcript path; Herdr keeps only the id, answering `kind: "id"` even when
  both were reported, so a reader that wants the file finds it by id. Auto Title
  reads it from the snapshot — no extra request — and what it does with it is in
  [title resolution](./title-resolution.md).
- **`pane.report_metadata` is how anything outside Herdr sets `PaneInfo.title`.**
  Probed directly: `herdr pane report-metadata <pane> --source X --title T` put
  `T` in the snapshot's `title` and a tab bearing it appeared within one poll;
  `--clear-title` undid it. Nothing installs a source for it today, which is why
  `title` is null in practice. It also carries `--display-agent`,
  `--state-label`, `--token` and a `--ttl-ms`.
- **`agent_status` is `idle | working | blocked | done | unknown`.** Every pane
  carries one, and a pane with no agent reports `unknown`. `TabInfo` carries one
  as well, aggregated over the tab's panes: with a single Claude Code pane
  working, its tab reported `working` while every other tab reported `unknown`.
  How it aggregates two agent panes in one tab has not been probed.
- **`TabInfo.number` is not the label an unnamed tab carries.** `number` counts
  every tab its workspace has ever held and never repeats — a workspace holding
  six tabs was seen numbering them 2, 9, 30, 33, 35, 36. The label Herdr puts on
  a tab nobody has named is its *position* in the workspace, counted from one,
  and it slides down whenever a tab to the left of it closes: three fresh tabs
  labelled `5`, `6`, `7` became `5`, `6` when the middle one was closed. Tabs
  arrive from `session.snapshot` in the order they are shown, so the position is
  the count of the workspace's tabs up to and including that one.

  Reading `number` as the default label locked every tab created after startup;
  the story is in [manual rename protection](./manual-rename-protection.md).
- **A tab has two unnamed shapes, and only one of them is the position.** A tab
  nobody has named reports its position (`herdr tab create` in a four-tab
  workspace answered `label: "5"`), but clearing a name stores exactly what it
  was given: `herdr tab rename wG:tS ""` answered `label: ""`, and the snapshot
  reported the same. The tab bar shows the position for both. Anything reading
  the label to mean "unnamed" has to accept the empty string as well.
