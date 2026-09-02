# CLAUDE.md

Herdr Auto Title — a Herdr plugin, written in Go, that generates tab titles from
each tab's current context. Long-running process that polls the Herdr session,
no LLM and no external service.

## Language rule (mandatory)

**Everything written into this repository is in English.** Code comments, commit
messages, log and error messages, documentation, test names, ticket text — all
English, with no exceptions. This holds regardless of the language the request
was made in; only the conversation with the user follows the user's language.

## Commit convention (mandatory)

Commits follow [Conventional Commits](https://www.conventionalcommits.org):

```
<type>(<optional scope>): <subject>

<optional body explaining why, wrapped at 72 columns>
```

Types in use: `feat`, `fix`, `docs`, `test`, `refactor`, `perf`, `chore`.
Scope is a package or area (`resolver`, `state`, `herdr`, `app`).

- Subject in the imperative mood, lowercase, no trailing period, ≤72 characters
  ("add manual rename protection", not "Added manual rename protection.").
- The body explains why, not what — the diff already says what.
- One logical change per commit.
- Never add a co-author trailer.

## Type rule (mandatory)

**A struct field exists only if code reads it.** Herdr's wire objects carry far
more than Auto Title needs; mirroring them in full makes a type claim a
dependency the code does not have, and every unread field is a promise to keep
something working that nothing exercises. Add a field when the code that reads
it lands in the same change, and delete a field the moment its last reader goes.
The same holds for methods, constants and event payload types.

## Script rule (mandatory)

**Everything in `scripts/` is Python 3 and uses the standard library only.**
Shell stays where it belongs: the one-line recipes in the Makefile. Anything
with a loop, a branch or a data structure is a Python script.

Two scripting languages in one repository means two sets of portability traps to
remember — `stat -f` against `stat -c`, `trap` against signal handlers, quoting
rules that differ per shell — for tooling nobody should have to think about.
Python was already here for the probes, so it is what the rest is written in.

Each script is executable, opens with `#!/usr/bin/env python3` and a module
docstring saying what it is for, and takes no dependency outside the standard
library.

## Comment rule (mandatory)

**A comment is at most three lines**, in every language in the repository, and
it says what is surprising rather than what is visible. A decision that needs a
paragraph goes in [docs/architecture](docs/architecture/), with one line in the
code pointing at it.

The rule in full, in Go terms and with worked examples of what to keep and what
to delete, is in [.claude/rules/comments.md](.claude/rules/comments.md).

## Commands

```sh
make            # list every target
make check      # fmt + vet + lint + test   ← the gate before any commit
make lint       # golangci-lint, pinned in tools/go.mod
make test       # go test -race ./...
make run        # build and run in the current Herdr session, DEBUG logging
make dev        # the same, restarting on every source change
make ps         # show running plugin/watcher instances
make stop       # stop them
make tabs       # current tab names
make watch-tabs # ...refreshed every second
make probe-snapshot # the session snapshot the plugin polls
```

`go test -race` is the gate, not `go test`: the poll loop and the change history
it keeps are exercised concurrently in tests, and a future reset action will
touch that history from outside the loop.

The linter lives in `tools/go.mod`, a module of its own, so its dependency tree
stays out of the plugin's: the main module keeps two dependencies and still
builds on Go 1.24, which is what Herdr needs at install time. `errcheck` is off
— the places that swallow an error say why they do.

## Herdr socket API — verified facts

The originating specification is wrong on several protocol details. These were
verified against Herdr 0.8.2, protocol 20. **Probe before assuming anything not
listed here** (`make probe-*`, `scripts/probe.py`).

- NDJSON over the socket at `HERDR_SOCKET_PATH`. Requests are
  `{"id","method","params"}` — `params` is required even when empty.
- **One request per connection.** Herdr closes the connection after answering,
  so every `Call` dials its own. Auto Title uses three methods and no others:
  `session.snapshot`, `pane.process_info` and `tab.rename`.
- **The event stream is not used, on purpose.** `events.subscribe` replays a
  backlog before anything live — about the last 95 revisions of *every* pane at
  ten a second, so ~10 s of history per active pane, closed panes included — and
  live events queue behind it (measured: a change made 2 s after subscribing
  arrived 13 s later). There is no cursor: `events.subscribe` takes only a
  subscription list, envelopes carry no timestamp or ordinal, and no method
  exposes a stream position. A snapshot costs 0.47 ms and 6 KB for six panes and
  describes the present. **Do not reintroduce a subscription** without measuring
  again and recording the result here.
- Subscription types would be dot-separated (`pane.updated`) while the events
  they deliver arrive snake_case (`pane_updated`), wrapped as
  `{"event": ..., "data": ...}`. `pane.output_changed` is a real event kind but
  is not an accepted subscription type; `pane.agent_status_changed`,
  `pane.scroll_changed` and `pane.output_matched` are per-pane and need a
  `pane_id`.
- `PaneInfo.title` is the agent's own title. It was null for every Claude Code
  pane observed; that agent reports its topic through `terminal_title_stripped`.
  `pane.report_metadata` sets it from outside Herdr (probed: a reported title
  reached the snapshot and a tab within one poll), but nothing installs a source
  for it today.
- `PaneInfo.agent_session` says which conversation a pane's agent holds, and is
  null until that agent's integration reports one.
  `herdr integration install <agent>` installs the hook (`herdr integration
  status` lists all seventeen). Claude Code's runs on `SessionStart` and calls
  `pane.report_agent_session`; Herdr keeps only the id, answering `kind: "id"`
  even when a transcript path was reported too. It arrives in the snapshot, so
  reading it costs no request.
- `agent_status` is `idle | working | blocked | done | unknown`; a pane with no
  agent reports `unknown`. `TabInfo` carries one too, aggregated over the tab's
  panes: with a single Claude Code pane working, its tab reported `working`
  while every other tab reported `unknown`. How it aggregates two agent panes in
  one tab has not been probed.
- `tab.rename` costs 0.16 ms median and 0.21 ms at p95 over forty calls —
  cheaper than the snapshot that precedes it.
- **A tab label is one line.** `tab.rename` accepts a newline and stores it
  verbatim — no error, no stripping — but the tab bar renders a single line, so
  a two-line label is not available. Herdr exposes no tab-bar height setting.
- **`PaneInfo.cwd` is the pane's own shell, not what the user is typing into.**
  A subshell moves `foreground_cwd` and leaves `cwd` behind: probed with
  `chezmoi cd`, which runs `$SHELL` in the source directory, the pane reported
  `cwd: ~/Work/global-sso` and `foreground_cwd: ~/.local/share/chezmoi` for as
  long as that subshell lived. Neither field is the pane's directory on its own
  — see the next two.
- **`foreground_cwd` is the deepest descendant's directory**, not the foreground
  process's own: a pane running `python3` that had spawned `sleep` in `/tmp`
  reported `foreground_cwd: /private/tmp` while `cwd` stayed on the pane's
  directory. Anything a program starts elsewhere takes it with it: an agent
  whose MCP server sits in `/tmp` reported that as the pane's.
- **A pane's directory is its foreground process's own `cwd`**, which only
  `pane.process_info` reports. Measured on a live session: an agent pane held
  `cwd: ~/Work/herdr-auto-title` (the shell, left behind) and
  `foreground_cwd: ~/Work/self-care-portal` (an MCP server), while the agent
  process itself was in `~/Work/self-care-portal` — the project the user was
  working in. The snapshot's pair is the fallback for a pane nothing was read
  for.
- **`foreground_processes` is the pane's foreground process group**, by the
  group id `pane.process_info` reports and the pane's controlling terminal. An
  agent's MCP servers are in it; a descendant started in a group of its own
  without a terminal, as Claude Code starts the commands it runs, is not.
- `PaneInfo` carries no foreground process name; that needs `pane.process_info`,
  one request per pane at 0.11 ms — cheaper than the snapshot, but one per pane:
  on an eight-pane session the reads measured 0.17 ms each against a 1.35 ms
  snapshot, so making one every poll cost as much again as the snapshot itself.
  Its `foreground_processes` lists the pane's foreground process *and its
  descendants*, deepest first, each with `pid`, `name`, `argv0`, a nullable
  `argv`, `cmdline` and `cwd`; the pane's entry carries `shell_pid` and
  `foreground_process_group_id`.
- Pane revisions are monotonic, which is how a poll tells which panes drew.
- **A revision does not track what is running in the pane.** Measured over ten
  minutes of a live eight-pane session: the foreground processes changed nine
  times and the revision moved with them only four. A pane running a build went
  `env` → `node` → `esbuild` → `fish` while its revision held at 10 throughout.
  A revision says the pane drew, which starting a command usually but not
  always provokes, so it is a cheap hint that a process read is due and never a
  promise that one is not.
- **`TabInfo.number` is not the label an unnamed tab carries.** It counts every
  tab the workspace has ever held and never repeats (a six-tab workspace
  numbered 2, 9, 30, 33, 35, 36). Herdr labels an unnamed tab with its
  *position* in the workspace, counted from one, and that label shifts down when
  a tab to its left closes. The snapshot lists tabs in display order, so the
  position is their count within the workspace.
- **An unnamed tab reports one of two labels.** A tab nobody has named carries
  its position, but `tab.rename` with an empty label stores the empty string and
  the snapshot reports it. Both render as the position in the tab bar, so code
  reading the label to mean "unnamed" must accept either.
- **A plugin the server starts inherits the server's environment**, not the
  shell of whoever installed it, which is why `HERDR_AUTO_TITLE_*` settings
  arrive through `config.env` (see
  [docs/architecture/configuration.md](docs/architecture/configuration.md)).
  Herdr creates `~/.config/herdr/plugins/config/<plugin id>/` and prints it from
  `herdr plugin config-dir <id>` and `herdr plugin list` (confirmed directly),
  and the 0.8.2 binary names `HERDR_PLUGIN_ROOT`, `HERDR_PLUGIN_CONFIG_DIR` and
  `HERDR_PLUGIN_STATE_DIR` for a plugin process — that half is read out of the
  binary's strings, **not observed on a live plugin**, because seeing it needs a
  `herdr server stop`. Auto Title uses its own directory and depends on none of
  them.
- `tab.get` and `pane.get` read one object each; `pane.list` filters by
  workspace only, not by tab. Neither is needed while the snapshot is one call.

Keep this list current: when a probe teaches you something new, add it here and
to [docs/architecture/herdr-socket-api.md](docs/architecture/herdr-socket-api.md),
which carries the same facts in full.

## Working here

- Work from the issue the change belongs to; [CONTRIBUTING.md](CONTRIBUTING.md)
  says when one is needed. If an issue turns out to rest on something false
  about Herdr, correct it there rather than silently working around it.
- Development runs against the user's real Herdr session, so **their tab names
  change while you work**. Run the plugin in the foreground, never in the
  background, and check `make ps` when something behaves oddly.
- **Decide from freshly read state.** Every poll reads the session and throws
  the result away again. What is carried between polls is only what a snapshot
  cannot say: when each pane last changed, what it was running when it was last
  asked — reused only until that pane's revision moves, and for no longer than
  `processRefresh` either way, because a revision does not track what runs in a
  pane — and how far each agent transcript has been read, because a transcript
  only grows and re-reading megabytes twice a second to find one new line would
  cost more than the rest of the loop together.
- Never pass terminal-derived values to a shell. Renames go over the socket API.
- How the plugin works and why — the poll loop, title resolution, sanitizing
  untrusted values, manual rename protection — is in
  [docs/architecture](docs/architecture/). Record a design decision there rather
  than in the README, which is for people using the plugin.
- The full workflow is in [docs/development.md](docs/development.md);
  installation and configuration are in [README.md](README.md).
