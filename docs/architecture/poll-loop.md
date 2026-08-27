---
type: doc
title: 'The Poll Loop'
description: 'Why Auto Title polls the Herdr session instead of subscribing to it, what one poll does, what little state survives between polls, and how the loop behaves when Herdr is unreachable.'
tags: [architecture]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# The Poll Loop

Auto Title is one long-lived process that reads the whole Herdr session twice a
second and renames the tabs whose titles no longer fit. There is no scrollback
scanning, no LLM and no external service.

```
                    every 500 ms
                          │
                          ▼
                  session.snapshot ──► whole session, one request
                          │
                          ▼
              which panes changed? (revisions)
                          │
                          ▼
              per tab: pick the context pane
                          │
                          ▼
                  deterministic resolver
                          │
            title differs? ──no──► nothing
                          │
                         yes
                          ▼
                     tab.rename
```

The loop lives in `App.Run` and `App.poll` (`internal/app/app.go`).

## Polling, not events

Herdr has an event stream and Auto Title ignores it, because subscribing replays
about ten seconds of history per active pane before delivering anything live and
offers no cursor to skip it. The measurements are in
[the socket API](./herdr-socket-api.md#why-the-event-stream-is-not-used).

A snapshot describes the present, costs one request whatever the session holds,
and carries every field the resolver reads. At 0.47 ms and 6 KB for six panes,
two polls a second come to about a thousandth of a core.

## Decide from freshly read state

**Every poll reads the session and throws the result away again.** A tab's
name is derived from the state read for that poll, which is what makes the
resolver's determinism worth anything: identical session state always yields an
identical title.

Four things are carried between polls, and each exists because a snapshot
cannot express it:

- **When each pane last changed** (`internal/state/changes.go`). A snapshot says
  what a pane holds but not when that became true, and a tab with several panes
  and none focused is named after whichever moved last. Herdr's pane revisions
  are monotonic, so comparing one poll's revisions with the last says which panes
  moved. The map is rebuilt from each snapshot, so panes that closed disappear
  from it for free.
- **What each pane was running** (`internal/state/changes.go`, kept beside the
  revisions). `PaneInfo` carries no process name, so naming a pane after the
  program in it costs a `pane.process_info` request per pane. Measured against
  an eight-pane session, a read is 0.17 ms and the snapshot before it 1.35 ms,
  so making one for every pane every poll cost as much again as the snapshot —
  and on a session where nothing is happening, every one of those reads returns
  what the last already said. A pane whose revision has not moved is running
  what it usually was, so its answer is reused until the revision moves. That
  test is a hint rather than a guarantee, and it was measured to be one: over
  ten minutes of a live eight-pane session the foreground processes changed nine
  times and the revision moved with them only four, one pane going
  `env` → `node` → `esbuild` → `fish` with its revision held at 10 throughout. A
  revision says the pane *drew*, which starting a command usually but not always
  provokes. So the reuse is bounded twice: by the revision, which catches the
  common case in the very next poll, and by `processRefresh` (2 s), which is
  what actually bounds how wrong the answer can be.

  Only the pane a tab is named from is asked about, so the request is per tab
  rather than per pane. The reuse still earns its keep: focus moves between the
  panes of a tab, and what was read for one is still there when it comes back.
- **How far each agent transcript has been read** (`internal/claude/transcript.go`).
  A transcript is append-only, so a poll reads the bytes appended since the last
  one rather than the file: a session that has run all day is megabytes, and
  re-reading it twice a second to learn the one line that changed would cost
  more than every other read in the loop together. What is kept per session is a
  path, a byte offset and the topic read so far. Sessions the snapshot no longer
  holds are dropped the same way panes are.

  A session whose transcript is *not* found is remembered too. Herdr can name a
  session before the agent has written a line of it, so the search has to be
  repeated — but finding the file means scanning every project directory the
  user has, and doing that twice a second for the life of a pane costs more than
  every other read in the loop. A failed search is therefore left alone for
  `locateRetry` (10 s).
- **What Auto Title last named each tab** (`internal/state/manual.go`), which is
  how a rename by the user is told from the plugin's own work. That is a design
  of its own: [manual rename protection](./manual-rename-protection.md).

One consequence worth stating: **the interval is the rename rate.** A tab
changes name at most once per poll however fast its pane is churning, so
`HERDR_AUTO_TITLE_POLL_MS` is both the freshness knob and the calm knob.

## One poll

1. `session.snapshot` — the whole session in one request.
2. `Changes.Observe` — note which panes' revisions advanced.
3. `Manual.Retain` — drop bookkeeping for tabs the session no longer holds,
   and release a lock whose tab has moved on. This runs off the snapshot's own
   labels, because it is what decides which tabs the next step can skip.
4. `tabsIn` — assemble tabs with their panes from the snapshot alone. Nothing
   is read here: assembly is what says which pane will be asked about.
5. Per tab: skip it if locked, otherwise read the one pane the tab is named
   from (`readInto`), resolve a title, check whether the label moved under us,
   and rename when the result differs from the label the tab already carries.

**Only the pane that names its tab is read**, and only while its tab is
nobody's. `pane.process_info` is asked about the panes that moved since they
were last read, reusing the last answer for the rest; a pane whose processes
cannot be read simply has none, and a failed read is not remembered as an
answer. That pane's directory is read for the branch it has checked out, every
poll and with nothing kept between polls: two small file reads at 0.038 ms are
cheaper than the bookkeeping that would keep a stale answer. Inside the poll the
read is memoized by directory, which is what stops the tabs of one project
walking the same tree once each — see
[title resolution](./title-resolution.md#the-git-branch).

The reads are left out rather than made and discarded because a tab is named
from one pane (`SelectContextPane`) and a locked tab is not named at all. A
four-pane tab therefore costs one process request rather than four, and a
session the user has named by hand costs the snapshot and nothing else. The
choice of pane is made from state the snapshot already carries — focus, agent
status, and which pane last drew — so it can be made before anything is read,
and the resolver arrives at the same pane on its own.

**Deduplication is what keeps the loop quiet.** The snapshot reports each tab's
current label, and a rename is skipped when the resolved title already equals
it — which is also what stops a rename from provoking the next one. A session
where every tab already carries the right name, and whose panes are sitting
still, issues nothing beyond the snapshot itself.

The whole poll is bounded by `pollTimeout` (5 s). A tab that closed between the
snapshot and its rename answers `tab_not_found`, which is expected rather than an
error.

## When Herdr is not there

A connection lives for exactly one request, so there is nothing to reconnect and
no connection state to reconcile. An outage is simply a run of failed dials, and
recovery is the first dial that succeeds.

- **No poll failing is fatal, the first one included.** Herdr launches a plugin
  through a one-shot startup hook rather than supervising it, so a plugin that
  gave up would stay dead for the rest of the session — and Herdr's socket can
  be a moment behind the process it just launched. Nothing carried between polls
  is spoiled by a failure, so the next tick simply tries again.
- **Polls keep their usual rate through an outage.** A failed dial to an absent
  socket costs microseconds, and the rate is what makes recovery immediate.
- **The logging backs off instead.** At two polls a second, an hour of Herdr
  being down is seven thousand identical warnings. `failureLog`
  (`internal/app/failures.go`) reports the first failure and then only as the
  run of them doubles, which turns that hour into thirteen lines, and a line on
  the way out says how many polls were missed.

The only failure that stops the process is a missing `HERDR_SOCKET_PATH`, caught
in `herdr.New` before the loop is ever entered.

Measured on a live session: with the socket removed for eight seconds the
process stayed up and polls resumed the moment it returned; started with no
socket at all, it stayed up for ten seconds on five warnings and named every tab
as soon as the socket appeared.

## Shutdown

`signal.NotifyContext` in `cmd/herdr-auto-title/main.go` cancels the context on
`SIGINT` and `SIGTERM`; `Run` returns and the process exits. There are no
debounce timers to cancel and no socket to close, because a connection never
outlives the call that made it.

Every source resolves synchronously inside the poll, so when `Run` returns there
is nothing left running.
