---
type: doc
title: 'Manual Rename Protection'
description: 'How Auto Title tells a rename you made from one it made itself using nothing but successive polls, why the first poll is special, how locks survive a restart without stealing a stranger tab, and the one thing this design cannot do.'
tags: [architecture]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Manual Rename Protection

Rename a tab yourself and Auto Title leaves it alone from then on. The same
holds for a pane, which Auto Title names too unless that is turned off.

There is nothing to correlate a rename with. The plugin polls rather than
subscribing, so a rename is not an event that arrives but **a label that has
changed between two polls**. The whole design follows from that, and it lives in
`internal/state/manual.go`.

## The rule

A tab is the user's work when its label moved to something Auto Title neither
set nor would have set. Three things are compared on every poll
(`Claims.Observe`):

- **Current** — the label the snapshot reports.
- **Desired** — what the resolver would name the tab right now.
- **Seen** — the label Auto Title last observed or set for that tab.

A label equal to *Desired* is never the user's on a tab or a pane: it cannot be
told from Auto Title's own work, and it is harmless either way. (A workspace on
the poll it is judged on is the one exception, below.) A label equal to *Seen*
has not moved, so nobody did anything. Anything else moved, and whoever moved
it was not the plugin.

`Claims.Applied` records each successful rename, so the plugin's own work never
reads as the user's on the next poll.

## Two traps this design walked into

Both were found by running it, not by reading it.

### The first poll is not each tab's first sighting

**The first poll never locks a tab or a pane.** On startup almost every tab
carries a label that is not yet what the resolver would produce, and locking on
that would claim the whole session at once. A workspace is the one thing it does
claim; see [A workspace is claimed on sight](#a-workspace-is-claimed-on-sight-not-after-a-move).

Applied per *tab* rather than per *poll*, the same rule loses names. A tab
created and named faster than the next poll is first seen already carrying the
user's name — and the rule said not to lock it, so Auto Title renamed it over.

They are told apart by what Herdr calls a tab nobody has named. After the first
poll, a tab Auto Title has never seen is one that did not exist before, and a new
tab already carrying something other than its default label was named by whoever
made it.

### Herdr's default label is a position, not `TabInfo.number`

The default label was first read from `TabInfo.number`. It is not that.
`number` counts every tab a workspace has ever held and never repeats — six tabs
were seen numbered 2, 9, 30, 33, 35, 36 — while the label on an unnamed tab is
its **position** in the workspace, so that sixth tab was labelled `6`.

The guard therefore compared `6` against `36`, never fired, and **every tab
created after the first poll was locked the moment it appeared**, written to the
lock file, and never named again. From the outside Auto Title looked like it had
simply stopped working on new tabs.

The position is in no field: tabs arrive from `session.snapshot` in display
order, so it is their count within the workspace (`App.tabsIn`).

The label also comes back — when a tab is unnamed again, and when every tab
shifts one place left because a tab before it closed. So **a tab wearing its
default label is nobody's, seen before or not**, and that check runs ahead of the
others for every tab. The cost is a user who renames a tab to exactly the digits
of its position and is not protected, which is the same trade the *Desired*
check already makes.

There is a second shape of it. A tab nobody has named reports its position, but
**clearing a name stores the empty string** — `tab.rename` keeps exactly what it
is given, and the tab bar renders the position for both. Until that was probed,
clearing a tab's name read as a rename to a label Auto Title had never seen, so
the gesture a user reaches for to hand a tab back was the one that locked it for
good. An empty label is therefore nobody's too, on the same line as the
position.

### A rename can land after its call has failed

`Claims.Applied` runs only when `tab.rename` answers. A call can also give up —
the poll's deadline passes while Herdr is stalled — after Herdr has already
read the request, and Herdr then applies it whenever it gets to it. Measured, a
stalled server applied renames 3 to 18 seconds after receiving them, while
another plugin's tab bar command kept timing out.

By then the name wanted has often moved on: a tab to the left closed and every
position slid, or the poll that reads the late label was itself cut short. The
tab carries a label that is neither *Seen* nor *Desired*, the rule reads it as
the user's, and **the tab is locked on a stale number for good**. In the session
where it was found, ten of twelve locked tabs had a late rename as their last
one.

So the label of a rename whose request was sent but got no answer
(`herdr.ErrUnanswered`) is kept per tab (`Claims.Sent`), and a tab found wearing
one is Auto Title's, like one wearing *Desired*, and is renamed on. The label is
forgotten once it lands or the tab closes. Recording the label as *Seen* up
front instead would be wrong the other way: a rename that never lands would
leave the old label looking moved, and lock that.

Only a request that was sent is kept. A call Herdr answered with an error was
refused, not deferred. A call that failed before its request was sent never
reached Herdr: a dial refused, or one made once the poll's deadline had passed,
as it has for a rename right after a process read that stalled.
Kept all the same, its label would pass for Auto Title's, and a user who then
chose that name would have it renamed away.

## Locks outlive the process, guarded by the label

Herdr can restart a plugin mid-session, and losing every manual name to that is
a worse surprise than the plugin briefly stopping. Locks are therefore persisted
(`manual-names.json`, written through a temporary file and renamed into place).

But Herdr's tab ids belong to a session, so a stored `wE:t2` may be an unrelated
tab by the time it is read back. **A lock records the label it was taken with**
and is released if the tab no longer carries it (`Claims.Retain`, which also
drops everything about tabs the session no longer holds).

The cost of that guard is stated plainly: **a rename made while the plugin is
not running is not remembered.** It cannot be told apart from an id reused by a
different session, and wrongly locking a stranger's tab is worse than forgetting
a rename made in the seconds the plugin was down.

## Handing a tab back

**Clear the tab's name and Auto Title takes it again**, on the next poll, with
the plugin running. Nothing implements that: `Retain` releases the lock because
the label moved off the one it was taken with, and `Observe` declines to take a
new one because an unnamed tab is nobody's. Renaming the tab to its position
does the same thing for the same reason.

Renaming it to anything else does **not** hand it back. The lock is released and
retaken within the poll, now on the new label — which is what keeps a tab the
user renames twice theirs.

What remains impossible while the plugin runs is releasing a lock without
touching the tab. The way to that is still to stop the plugin, remove the tab's
entry from `manual-names.json` (or delete the file), and start it again —
editing the file under a running plugin achieves nothing, because the locks live
in memory and the file is rewritten from them.

A `reset` subcommand talking to the running process over a control channel of
its own is specified and not built.

## Panes are protected the same way, and separately

Auto Title can name panes as well as tabs
([configuration](./configuration.md)), and a pane it names is a pane the user
can rename back. The rule above is the same rule: `Manual` holds one `Claims`
per kind — `Manual.Tabs`, `Manual.Panes` and `Manual.Workspaces` — and each
runs it over ids of its own. Locks for all three live in the same file, panes
under `locked_panes` and workspaces under `locked_workspaces`, so a store
written by an older version reads back unchanged. The one way the workspace
set differs is claimed on sight, below.

Two things differ, both because Herdr labels a pane differently from a tab.

**A pane has one unnamed spelling, not two.** `pane.rename` *clears* an empty
label rather than storing it, and Herdr omits the field entirely until a pane
has a label, so an unnamed pane is `""` and nothing else. There is no position
to compare against and no `TabInfo.number` trap to walk into — the whole of
`PaneSightingFrom` is the pane's id, its label and what the resolver wants.

**Claiming a tab does not claim its panes.** The two are separate locks on
purpose: naming a tab by hand says what the tab bar should read, and says
nothing about how the panes inside it should be listed in the goto panel. A tab
the user has taken still has its panes named, and a pane the user has taken sits
inside a tab Auto Title goes on naming.

Handing a pane back is the same gesture with one spelling: clear its label
(`herdr pane rename <PANE_ID>`) and the next poll takes it again.

## Nothing expires

An earlier design had an expiring `ExpectedRename` to correlate a `tab_renamed`
event with the request that caused it. That race only exists when the answer
arrives separately from the question. A poll asks and is answered in the same
breath: either the label is the one Auto Title set, or it is not. What survives
of the idea is a single remembered label per tab, pruned to the live session on
every poll rather than by a clock.

## A workspace is claimed on sight, not after a move

Tabs and panes are protected once a label has been seen to move: the first poll
cannot tell the user's name from nobody's, so it claims nothing. A workspace
can be told apart on the first poll, and is.

Herdr labels a workspace nobody has renamed after the directory it holds, and
never revisits that label (see the socket API note). So `Sighting.Default` is
that basename, and `Claims.Observe` needs one thing tabs do not: any label that
is not the default is claimed the first time it is seen, rather than waited on —
even one the resolver would have chosen itself, which on a tab cannot be told
from Auto Title's own work but on a workspace was simply there first.
`Claims.lockUnseen` is that difference, and the workspace set is the only one
that carries it.

The two halves each cover what the other cannot. A workspace renamed while Auto
Title was not running is remembered by no poll, and the default test is what
stops it being taken. A workspace renamed to something the resolver might itself
have produced passes the default test, and the claim is what stops it.

A workspace opened after the first poll still wears its directory's name, so it
is taken like any other rather than counted as the user's.

The basename is read from a pane, and a tab can exist before its pane does. A
workspace that shows no directory on the first poll has no default to compare
against, so that poll cannot tell its label apart; it is judged by the same
test on the first poll that shows one, and until then is neither claimed under
whatever it wore nor named over it (`Claims.Pending`, which `App.apply` checks
after `Observe`), and stays unseen.

A row this plugin wrote outlives it too. Herdr runs the startup hook again at
every start and live handoff, and a restarted plugin finds that row wearing
neither the basename nor a name it remembers — which the default test would
read as the owner's, and lock at a label the work has moved past. So the label
last written per workspace is kept in the same file (`written_workspaces`,
recorded by `Claims.Applied`), and the judging poll reads its own work as its
own: renamed on, not claimed. A rename that landed after its call got no answer
is recorded the same way once a poll sees it wearing the row. It is forgotten
with the workspace. A user who renames a row to exactly what the plugin last
wrote is not told apart, as a tab renamed to *Desired* is not; nor is a rename
Herdr applies only after the restart, which no lock file can have seen.
