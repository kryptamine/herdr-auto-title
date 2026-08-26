---
type: doc
title: 'Manual Rename Protection'
description: 'How Auto Title tells a rename you made from one it made itself using nothing but successive polls, why the first poll is special, how locks survive a restart without stealing a stranger tab, and the one thing this design cannot do.'
tags: [architecture]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Manual Rename Protection

Rename a tab yourself and Auto Title leaves it alone from then on.

There is nothing to correlate a rename with. The plugin polls rather than
subscribing, so a rename is not an event that arrives but **a label that has
changed between two polls**. The whole design follows from that, and it lives in
`internal/state/manual.go`.

## The rule

A tab is the user's work when its label moved to something Auto Title neither
set nor would have set. Three things are compared on every poll
(`Manual.Observe`):

- **Current** — the label the snapshot reports.
- **Desired** — what the resolver would name the tab right now.
- **Seen** — the label Auto Title last observed or set for that tab.

A label equal to *Desired* is never the user's: it cannot be told from Auto
Title's own work, and it is harmless either way. A label equal to *Seen* has not
moved, so nobody did anything. Anything else moved, and whoever moved it was not
the plugin.

`Manual.Applied` records each successful rename, so the plugin's own work never
reads as the user's on the next poll.

## Two traps this design walked into

Both were found by running it, not by reading it.

### The first poll is not each tab's first sighting

**The first poll never locks anything.** On startup almost every tab carries a
label that is not yet what the resolver would produce, and locking on that would
claim the whole session at once.

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

## Locks outlive the process, guarded by the label

Herdr can restart a plugin mid-session, and losing every manual name to that is
a worse surprise than the plugin briefly stopping. Locks are therefore persisted
(`manual-names.json`, written through a temporary file and renamed into place).

But Herdr's tab ids belong to a session, so a stored `wE:t2` may be an unrelated
tab by the time it is read back. **A lock records the label it was taken with**
and is released if the tab no longer carries it (`Manual.Retain`, which also
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

## Nothing expires

An earlier design had an expiring `ExpectedRename` to correlate a `tab_renamed`
event with the request that caused it. That race only exists when the answer
arrives separately from the question. A poll asks and is answered in the same
breath: either the label is the one Auto Title set, or it is not. What survives
of the idea is a single remembered label per tab, pruned to the live session on
every poll rather than by a clock.
