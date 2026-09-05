# AGENTS.md

Herdr Auto Title — a Herdr plugin, written in Go, that generates tab titles from
each tab's current context. Long-running process that polls the Herdr session,
no LLM and no external service.

## Repository layout

```
cmd/herdr-auto-title  the binary
internal/app          the poll loop, configuration, failure handling
internal/herdr        the socket client; herdrtest beside it is its stub
internal/state        a session snapshot turned into what each tab is doing
internal/resolver     that state turned into a title, one source at a time
internal/claude       what a Claude Code session is about, from its transcript
internal/git          what a repository has checked out, read from .git
scripts/              the Python probes
docs/architecture/    how the plugin works and why
```

Each package's doc comment says the rest.

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

## Branches and pull requests (mandatory)

**Never commit to `main`.** Branch from it first, named `<type>/<kebab-summary>`
with the types the commits use: `feat/optional-agent-name`,
`docs/security-policy`, `chore/tighten-the-linter-set`.

- **Pull requests are merged by rebase.** Merge commits and squashing are both
  disabled, and each broke something: GitHub puts the conventional PR title into
  a merge commit, so release-please counted every change twice, and a squash
  collapses a pull request into one changelog line, losing the granularity that
  "one logical change per commit" exists to produce.
- **`CHANGELOG.md`, the tags and the version in `herdr-plugin.toml` belong to
  release-please.** Never edit one by hand.
- Keep a pull request to one thing. A refactor, a feature and a formatting sweep
  are three pull requests.

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

## Herdr socket API — the traps

The originating specification is wrong on several protocol details. Everything
here was verified against Herdr 0.8.2, protocol 20. **Probe before assuming
anything** (`make probe-*`, `scripts/probe.py`).

The facts in full — the measured costs, the field inventories, the event kinds,
what every object carries — are in
[docs/architecture/herdr-socket-api.md](docs/architecture/herdr-socket-api.md),
which is the record. **A probe that teaches something new goes there.** Below
are only the facts that would otherwise mislead the code in silence.

- **One request per connection.** Herdr closes the connection after answering,
  so every `Call` dials its own. That is why nothing here reconnects.
- Auto Title uses three methods and no others: `session.snapshot`,
  `pane.process_info` and `tab.rename`.
- **Do not reintroduce an event subscription.** `events.subscribe` replays
  about ten seconds of backlog per pane before anything live, with no cursor to
  skip it, so a subscriber opens by reacting to a session that is gone. A
  snapshot describes the present and costs less than the rename that follows it.
- **No single field is a pane's directory.** `cwd` is the pane's own shell,
  which a subshell leaves behind; `foreground_cwd` is the _deepest
  descendant's_, so anything a program starts elsewhere takes the pane with it.
  The directory is the foreground process's own `cwd`, which only
  `pane.process_info` reports.
- **A revision does not track what is running in a pane.** Measured, the
  foreground processes changed nine times while the revision moved four. A
  revision says the pane drew: a hint that a process read is due, never a
  promise that one is not.
- **`TabInfo.number` is not the label an unnamed tab carries.** Herdr labels an
  unnamed tab with its _position_, which slides down when a tab to its left
  closes, while `number` counts every tab the workspace has held and never
  repeats. Reading `number` as the label once locked every tab made after start.
- **An unnamed tab reports one of two labels**: its position, or the empty
  string `tab.rename` stores verbatim when given one. Code reading the label to
  mean "nobody named this" must accept either.
- **A tab label is one line.** `tab.rename` takes a newline and stores it
  verbatim, but the tab bar renders one row and Herdr exposes no height setting.
- **`PaneInfo.title` is the agent's own title, and is null in practice.** Claude
  Code reports its topic through `terminal_title_stripped` instead, and
  `agent_session` stays null until that agent's integration is installed.
- **A plugin the server starts inherits the server's environment**, not the
  shell of whoever installed it, which is why `HERDR_AUTO_TITLE_*` settings
  arrive through `config.env` (see
  [docs/architecture/configuration.md](docs/architecture/configuration.md)).

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
- **The code is the source of truth, then `docs/architecture`, then a comment.**
  A doc that contradicts the code is a bug in the doc, so fix it in the change
  that found it rather than leaving the next reader to rediscover the same
  thing.
- Never pass terminal-derived values to a shell. Renames go over the socket API.
- How the plugin works and why — the poll loop, title resolution, sanitizing
  untrusted values, manual rename protection — is in
  [docs/architecture](docs/architecture/). Record a design decision there rather
  than in the README, which is for people using the plugin.
- The full workflow is in [docs/development.md](docs/development.md);
  installation and configuration are in [README.md](README.md).
