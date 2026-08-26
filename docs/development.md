# Development workflow

Auto Title is developed against the Herdr session you are already working in.
That is deliberate: the plugin's whole job is reacting to a real terminal, and a
synthetic session would not have agents running, directories changing or panes
churning. The cost is that **your own tab names change while you work on it** —
that is the plugin doing its job, not a problem to fix.

Everything below assumes `make`, Go 1.24+ and a shell running inside a Herdr
pane (which is what exports `HERDR_SOCKET_PATH`).

Run `make` on its own to list every target.

## The three loops

Work in the innermost loop that can answer your question. Most changes never
need the outer two.

### 1. Tests — seconds, no Herdr involved

```sh
make test        # go test -race ./...
make test-v      # the same, verbose
make check       # fmt + vet + lint + test, run this before every commit
```

The same four run in CI on every push and pull request
(`.github/workflows/ci.yml`), plus the Go version floor the manifest promises,
which a laptop on the newest toolchain does not cover. Windows was tried once
and dropped: the fixtures assume Unix paths, so `filepath.IsAbs` rejects every
directory in them and every tab falls back to `Shell`.

The suite drives the whole loop through `herdr.StubClient`: the first poll, a
tab appearing later, deduplication, rename failures, a poll that fails outright.
If a change can be expressed as "given this session, expect this title", it
belongs here and nowhere else.

**Write the test first when you are fixing something a live run revealed.** Every
defect found so far came from a live run and none from the happy path: a busy
pane starving its own timer, a tab closing between resolution and rename, and
the event backlog that eventually cost the event stream its place in the
design.

### 2. Live run — a minute, in your real session

```sh
make run         # build, then run in the foreground with DEBUG logging
```

Rules that are worth following literally:

- **Run it in a tab of its own, in the foreground.** Ctrl+C then always stops it.
- **Never background it.** A stray instance keeps renaming tabs long after you
  have moved on, and you will blame the wrong code for it.
- **`make ps` before you start, `make stop` when confused.** Both targets cover
  the watcher as well as the plugin. They exist because backgrounded instances
  really do survive — twice during this project, once because a `pkill` pattern
  did not match, and once because `make check` rewrote a file, which a forgotten
  watcher took as a cue to rebuild and restart.

Watch what it does from a second tab:

```sh
make watch-tabs  # tab ids and labels, refreshed every second
```

While iterating on the plugin itself, replace `make run` with:

```sh
make dev         # rebuild and restart on every source change
```

Same rules apply: foreground, one tab, Ctrl+C to stop both watcher and plugin.
Note that `make check` rewrites files with `gofmt -w`, so a watcher left running
in another tab will rebuild and restart on it.

### 3. Protocol probe — when you are about to assume something

```sh
make probe-snapshot  # the snapshot the plugin polls
```

`scripts/probe.py` talks to the socket directly, so it shows you the wire truth
rather than what this repo believes. Reach for it **before** writing code that
reads a new field or calls a new method.

This is not optional caution. The specification this project was planned from was
wrong about the socket in ways that each cost an afternoon: it serves **one
request per connection**, `PaneInfo` carries no process name, and — the one that
reshaped the design — subscribing to events replays about ten seconds of history
per active pane before anything live.

The current list of verified facts lives in the README under *Notes on the Herdr
socket API*, and the measurements behind the polling decision are in CLAUDE.md.
When a probe teaches you something new, add it there.

## Registering it with Herdr

`make run` starts the binary from your working tree and registers nothing.
Herdr itself knows two ways to hold a plugin, and it refuses to hold both at
once for the same id (`herdr.auto-title`).

While developing, link the checkout:

```sh
make build                                    # link runs no build command
herdr plugin link /path/to/herdr-auto-title
```

To use the published plugin the way anyone else does:

```sh
herdr plugin unlink herdr.auto-title          # installing over a link is refused
herdr plugin install kryptamine/herdr-auto-title
```

`unlink` only unregisters; your checkout is left alone. `install` takes GitHub
shorthand and nothing else (`owner/repo`, or `owner/repo/subdir`), clones the
repository, runs the build command from `herdr-plugin.toml` and registers what
it built. `herdr plugin list` shows which of the two you are running.

Neither one starts anything. Herdr runs `[[startup]]` when the **server**
restores a session, and has no hook for install or enable, so a freshly
installed or linked plugin sits idle until `herdr server stop` and a fresh
`herdr`. Opening a new terminal only attaches another client and starts
nothing — `herdr status server` reports the uptime that gives it away.

That is why `make run` exists: it is the only way to see your working tree do
something without taking the session down.

## Working through a ticket

Before changing how a title is decided, read
[architecture/title-resolution.md](architecture/title-resolution.md) — most of
what looks arbitrary in the resolver is a measurement that overturned something
the specification assumed.

Tickets live in `docs/issues/` and are ordered by dependency; take any whose
blockers are done.

1. Read the ticket. Probe anything it asserts about the API that is not already
   in the README's verified list.
2. Write the failing test for the behaviour the ticket describes.
3. Implement until `make check` is green.
4. Do one live run and actually look at the logs — the fast loop cannot see
   real churn, real title values, or what Herdr does under load.
5. Tick the ticket's acceptance criteria. If a criterion turned out to be based
   on something false, fix the ticket text rather than quietly skipping it.

## Reading the logs

`make run` sets `HERDR_AUTO_TITLE_DEBUG=1`. What each line tells you:

| Line | Meaning |
|------|---------|
| `starting auto title` | the poll interval and length limit actually in force |
| `tab renamed` | the only line that means Herdr was asked to do something |
| `poll failed` | a snapshot did not come back; the next tick retries, and a run of these is logged on a backoff rather than once per poll |
| `the session is answering again` | polls are working after a run of failures, and how many were missed |
| `rename failed` | something went wrong that is worth your attention |

A log with nothing after `starting auto title` is the plugin working correctly:
every tab already carries the name it should. The loop is deliberately silent
when it has nothing to do, because at two polls a second anything else would be
unreadable.

## Things that will bite you

**Your tabs get renamed and stay renamed.** There is no undo — the plugin does
not remember previous names. Manual rename protection is a later ticket; until it
lands, a name you set by hand is overwritten on the next context change.

**The poll interval is the rename rate.** A tab changes name at most twice a
second by default, however fast its pane is churning. If a tab renames more
often than you can read it, raise `HERDR_AUTO_TITLE_POLL_MS`.

**`config.env` is read once, and `make dev` does not watch it.** The watcher
rebuilds on `*.go` in the checkout, so a setting you change in the configuration
directory reaches the plugin only when the process starts again: Ctrl+C and
`make run`. Note that `make run` sets `HERDR_AUTO_TITLE_DEBUG=1` in the
environment, which beats whatever the file says about it.

**Short-lived tabs produce `tab_not_found`.** Herdr creates and closes tabs for
its own purposes between a snapshot and the rename it leads to. That is handled
and logged at DEBUG; if you see it as a warning, something regressed.

**Do not reach for the event stream.** It looks like the obvious mechanism and
it is a trap: subscribing replays about ten seconds of history per active pane
before anything live, and there is no cursor to skip it. The measurements are in
CLAUDE.md. If you think you have found a way around it, measure and record the
result there before changing the design back.

**`go test -race` is the gate, not `go test`.** The change history is shared
state and a future reset action will touch it from outside the loop. Races here
surface as tabs that are occasionally named wrong, which is nearly impossible to
debug after the fact.
