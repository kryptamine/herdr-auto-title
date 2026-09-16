---
type: doc
title: 'Title Resolution'
description: 'How a tab becomes a name: which pane speaks for the tab, the confidence ladder the sources order themselves by, what each source contributes, the rules that keep a title from repeating itself, and why the tab position rides in front of it.'
tags: [architecture]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Title Resolution

A title reads as a path from the general to the particular, with one separator
throughout, behind the number of the tab it names:

```
3 · self-care-portal › nvim › auth.provider.ts
```

Where a part came from — a directory, a program, a file — is not something a
separator can convey, and a second one would only ask the reader to learn a
distinction they cannot see. So every part shares one, `Separator`
(`internal/resolver/sanitize.go`). The number in front is not a part of the
title, and [carries a mark of its own](#the-position-is-not-a-part-of-the-title).

Structurally a title is four fields, `Parts{Context, Branch, Agent, Activity}`
(`internal/resolver/resolver.go`), formatted in that order: *where* the user is
and *what* they are doing. The branch belongs to the first of those — it
qualifies the directory rather than standing beside it as a separate kind of
thing. The agent's name is a field rather than a prefix on the activity because
[the user decides whether it is shown](#the-agents-name-is-optional), and
because holding it apart is what lets the repetition check see the activity as
it is.

## One pane speaks for the tab

A tab holding several panes is named after one of them, never after a blend of
both. `TabFrom` (`internal/state/tab.go`) picks it once, as `TabState.Context`,
in order:

1. the focused pane;
2. failing that, a pane running an agent that is `working` or `blocked` — a
   split where the user left an agent running is about that agent, even though
   the pane below it saw the last update;
3. failing that, the pane that changed most recently.

Each rule takes the pane that changed most recently among those it accepts,
and ties break on pane ID, so identical state always yields the same choice.
Both halves of the name then come from that pane alone.

With `HERDR_AUTO_TITLE_PREFER_AGENT=true` a pane running an agent comes before
all three, so an editor opened beside the agent leaves the tab named after the
agent instead of flipping with focus. That rule takes an agent in any state,
unlike rule 2: a user who asked for it has said the tab is about its agent,
and an agent that finished is still what that tab was opened for.

## The confidence ladder

Each source states its own place, and the resolver sorts itself by those numbers
rather than by the order the sources happen to be listed in
(`internal/resolver/resolver.go`):

| Confidence | Source | File | Contributes |
|---:|---|---|---|
| 90 | Agent title | `agent.go` | Activity |
| 80 | Terminal title | `terminal.go` | Activity |
| 75 | Session transcript | `transcript.go` | Activity |
| 70 | Foreground process | `process.go` | Activity |
| 60 | SSH session | `ssh.go` | Context |
| 40 | Git branch | `git.go` | Branch |
| 30 | Working directory | `cwd.go` | Context |
| 10 | Generic fallback | `resolver.go` | the whole name (`Shell`) |

The tab's position is not on this ladder, and [why is below](#the-position-is-not-a-part-of-the-title).

**A source never overrides a field a higher-priority source already supplied**,
but a lower one can still complete the other half. That is why the working
directory at 30 fills the context of a title whose activity came from an agent
at 90.

The numbering lives in one block because a source's place is only meaningful
relative to the others, and the gaps are what make room for the next one.

## What each source knows

### Agent

Herdr recognizes agents directly, and their process lists do not: a coding agent
shows up as a `caffeinate`, several `node`s and an MCP helper, with its own name
nowhere among them. When an agent reports a title, that title is the most direct
statement of what a tab is for that Auto Title will ever see, so it outranks
everything.

In practice most agent context arrives one rung lower. `PaneInfo.title` was null
for every Claude Code pane observed; that agent reports its topic through the
terminal title instead. An agent that echoes its own name (`Claude Code`) is
rejected as an activity — it is compared against the agent Herdr recognized in
the pane rather than against a list — and reappears as a *kind*, so a tab reads
`dashboard › claude` until there is something to report and
`dashboard › claude › Implement OAuth scopes` after. That name can be [turned
off](#the-agents-name-is-optional).

### Terminal title

The richest source in practice, and the one that carries most agent context. Its
value is cleaned hard before it is trusted — see
[sanitization](./sanitization.md).

### The session transcript

An agent that has titled its terminal has already said what it is doing, sooner
and for free. This source answers when it has not, which is a real and repeatable
case rather than an edge: Claude Code derives its terminal title from the user's
own prompts, so **a session opened with a slash command and answered by the
agent alone never gets one**. Its tab read `3 · claude` however long it ran.

Herdr's own integration hook is what makes the transcript reachable.
`herdr integration install claude` writes a `SessionStart` hook that reports the
session id through `pane.report_agent_session`, and it arrives in the snapshot
as `PaneInfo.agent_session` — no extra request. Without the hook the field is
null, this source declines, and every other rung works as it always did.

The transcript is then read from disk (`internal/claude/transcript.go`), and two
lines in it can name a session:

- `ai-title`, the title Claude Code generates and puts in its terminal title.
  The last one wins — a session is renamed as it goes.
- failing that, the **first prompt the user actually typed**, which is what
  Claude Code's own session list shows for an untitled session. A prompt that is
  a slash command yields the command and what it was called with
  (`/code-review spec.md` → `code-review spec.md`): the argument is usually what
  tells one run of a command from the next.

Telling the user's prompt from the rest is not cosmetic. A slash command expands
into the conversation as further user messages, and a resumed session opens with
a caveat block written by the tool; either would name a tab after the plumbing.
The transcript marks what the user typed with `origin.kind: "human"`, and only
those lines are read.

Three costs are worth stating plainly:

- **It reads what the user said to their agent.** That is why it can be turned
  off with `HERDR_AUTO_TITLE_TRANSCRIPT=false`, and why nothing else in the
  plugin reads a file it was not pointed at.
- **The format is undocumented.** `ai-title` and `origin.kind` are Claude Code's
  internals and can change in any release. A transcript that no longer carries
  them yields nothing and the source declines — the failure mode is the tab
  named as it was before this existed, not a wrong name.
- **The session id becomes part of a path.** It arrives over the socket, so it
  is refused unless it is shaped like a UUID rather than cleaned.

### Foreground process

Only a lone process names a pane. An editor reports as `nvim`; a build tool
reports as `esbuild` and five `node`s, and picking one of those would be
guesswork when the pane's terminal title already says what it is doing. A shell
as the foreground process means there is no activity, not that the shell is the
name.

What this source produces is a **kind** — the program, not the work.
`qualify` binds a kind to whatever a higher source found, and `stripKind` drops
a kind a detail already carries, so `nvim › auth.provider.ts - Nvim` does not say
the same thing twice. A kind with nothing left to add stands alone:
`dashboard › nvim` for an editor with no file open. An agent is the one kind
that is not bound here — only stripped — because it is a field of its own, which
the user can switch off.

A mapping from command lines to friendlier names (`yarn dev` → `Dev`) was
specified and is deliberately not built: the commands it would map are invisible
in the process table, visible only in the terminal title, and a source below the
terminal title can never fill an activity the terminal title has already filled.

**On Windows this source and the ssh one below are mostly silent.** Herdr lists
only the pane's shell or a recognized agent as what a pane there is running —
see [the socket API](./herdr-socket-api.md) — so an editor or an ssh session
never reaches either source, and its tab is named from the terminal title and
the directory alone. Process names arrive there with an `.exe` the state package
strips, so `pwsh.exe` is read as the shell it is.

### SSH

A pane running `ssh` is named after the machine it reached, not the directory it
was launched from: `ssh › prod-01`, and
`ssh › prod-01 › Restart the queue workers` once the remote shell has something
to report.

**The mark goes on the host rather than into the activity slot**, because the
activity is contested — a remote shell sets a terminal title, that title
outranks anything this source could put there, and the tab would stop saying it
is remote at exactly the moment it has most to say. Nothing else names a
machine, so the host slot has no such competition.

The user is dropped: `root@prod-01` and `deploy@prod-01` are the same machine,
and a tab bar has no room to say who is logged in. Options are parsed rather
than guessed at, so `ssh -p 2222 prod-01` and
`ssh prod-01 tail -f /var/log/syslog` both yield `prod-01`.

A destination that cannot be read leaves the mark standing alone, as `ssh`,
rather than letting the working directory take the context. It briefly did the
opposite — with no host to bind to, the mark went into the activity slot — and
that put it back in the contested half, where the remote shell's own title
outranked it and a remote tab read exactly like a local one.

**Only the pane's foreground process marks it remote**, the last one
`pane.process_info` lists. Everything before it is a descendant, and programs
start ssh of their own: `git push` to a GitHub remote, Claude Code checking it
can reach GitHub, and ssh itself for a `-J` jump host. When any ssh in the list
counted, an agent's tab read `ssh › github.com` while the check ran, and a jump
named the tab after the bastion rather than the destination.

While ssh connects, the terminal title is still the local shell's, and a shell
that titles its window with the command it runs says `ssh root@prod-01` — which
read `ssh › prod-01 › ssh root@prod-01` until the remote prompt replaced it. A
title opening with the word `ssh` in a pane running ssh is therefore not an
activity. It is recognized by that word rather than by the host because fish
trims the command to twenty columns, leaving `ssh deploy@productio`.

### The git branch

The branch a pane is on, read from the files under `.git` and never by running
git: `git rev-parse` measured 12.37 ms against 0.019 ms for reading `HEAD`, on a
poll whose whole snapshot costs 0.47 ms. The reading is not cached between polls
— at 0.038 ms including the walk up to the repository, a fresh answer costs less
than remembering a stale one, and a checkout shows up in the tab within one
poll. It is read only for the panes the poll reads at all, which without pane
naming is the one pane each tab is named from ([the poll loop](./poll-loop.md)).

**Which directory the branch is read from is not the pane's alone.** A coding
agent sent into a worktree never moves the pane's directory: it enters the
worktree internally, so the branch came back from the repository root as the
trunk, and the trunk contributes nothing — fourteen agent panes on fourteen
feature branches read alike. Both checkouts are therefore read — the pane's own
directory and the agent's — and the agent's answer is preferred when it belongs
to the same repository and has something to say. It is an override with a
fallback rather than a replacement, so a pane never loses a branch it used to
show: where the agent has nothing usable to say, the answer is the one this
source always gave.

Where the agent is working comes from the session transcript already opened for
its topic, as `claude.Topic.Dir` (`internal/claude/transcript.go`), and is `""`
when the transcript names none — no extra read, and nothing is opened that was
not going to be read anyway. The transcript carries a `cwd` on its `user`,
`assistant`, `system` and `attachment` lines but not on `ai-title`,
`last-prompt`, `mode` and several other types, so the reader remembers the last
line that carried one. It is reported as the transcript spelled it, and only an
empty value is refused, because that would erase the directory the session last
named; whether a path can be a checkout at all is settled where one is read. It
is cleared by the truncation reset and
survives the lost-path reset, exactly as the topic does — a transcript that was
replaced says nothing until it has been read again, and one whose file has gone
missing still says what it last said.

**The choice happens after both are labelled**, in `Git.Resolve`
(`internal/resolver/git.go`): the pane's checkout is labelled, and a non-empty
label from the agent's checkout replaces it when
`git.Checkout.SameRepository` holds. Labelling first is the whole reason there
is no second set of rules. An agent on a trunk labels to `""` and loses without
anything here testing for a trunk; a detached agent labels to its short hash and
wins without anything here testing for detachment. An earlier version decided
between the two *checkouts*, before either had a label, and had to re-derive
both of those facts to do it.

That equivalence has one limit worth stating: `label` suppresses a branch only
when it equals the repository's recorded default, so where a repository records
none — it has no remote, or none has been fetched — an agent standing on `main`
labels non-empty and wins, taking the tab from the pane's own branch. The same
mechanism is what finally shows the agent's branch in such a repository, which
is what this source exists for, so the two arrive together.

`SameRepository` compares `CommonDir`, the directory holding the refs a
repository shares with its worktrees, identical for a repository and every
worktree of it. A checkout outside any repository carries none, so a pane
outside one has nothing to match and keeps its own answer — the boundary of this
source rather than a case to widen. Two such checkouts do compare equal, and are
stopped by the agent's label being empty rather than by a guard, which would be
one that could never change an answer.

**A directory that is gone is refused where directories are read**, in
`git.discover`, beside the empty and relative paths it already rejects. A
worktree is removed while a transcript that named it keeps saying so, and the
walk up from a path that no longer exists lands on the repository above it —
whose branch is a real one, and would stand in for the pane's. That is not this
source's problem alone: a shell left in a deleted directory reports its parent's
branch the same way.

It catches a path that is gone, and only that. A directory that outlived its own
`.git` file still exists, so the walk up still answers with the repository above
it — a shape no check here can tell from a legitimate subdirectory of a
checkout, which this source deliberately resolves upward.

**The two common directories are compared cleaned and not resolved.** Both sides
come out of `git.discover` (`internal/git/git.go`), which already cleans the path
it walks from, so the comparison is exact on two clean absolute paths. Where a
checkout is reached through a symlink or an alias on one side only, the two
spellings differ and `SameRepository` is false: the pane keeps its own branch,
which is today's behaviour and never a wrong label. Resolving symlinks instead
would cost two extra stat-walks per pane per poll on the hot path, against a
case where the feature merely does not activate. It is a deliberate trade, not
an oversight.

**The context does not follow the agent — only the branch does.** Two reasons,
and the second is the sharper one. A worktree's directory names nothing worth
leading a title with: `annotation-drift` says nothing about the project `vrms`,
which is what the pane's own directory still says. And the rule below, that [a
branch equal to its context is
dropped](#a-worktree-does-not-say-its-branch-twice), would then delete the branch
outright — a `.claude/worktrees/<branch>` directory's basename *is* the branch, so
context and branch would arrive identical and the segment this source exists for
would silently go.

*Within* one poll the answer is memoized by directory (`checkoutMemo`,
`internal/app/reads.go`), because the tabs of a project usually share one: six
tabs of the same checkout walked the same tree six times, and a memo that is
thrown away with the poll that filled it cannot hand back a stale answer, which
is the only thing the refusal to cache between polls is about. A pane whose agent
sits in a different directory costs a second walk, so a poll walks once per
**distinct directory** rather than once per tab — and two panes whose agents
share a worktree still collapse to one.

A branch says which slice of a project a tab is on, so it qualifies the
**context**: `dashboard › feat/oauth › nvim › auth.ts`. Three rules keep it from
saying anything it has not earned:

- **The trunk contributes nothing.** Which branch that is comes from the
  repository itself, `refs/remotes/origin/HEAD`, rather than from a list of
  names: a team whose trunk is `develop` gets the same silence, and a branch
  actually called `main` off a `develop` trunk still shows. A repository that
  records no default shows its branch always.
- **A name that fits is left whole.** `feat/oauth` keeps the namespace that
  tells it from `fix/oauth`. Only a name too wide for `BranchMax` is reduced,
  and then an issue key wins outright (`bugfix-asa-cpanel-uapi-mc-13675` →
  `MC-13675`) because it identifies the work whatever convention wraps it;
  failing that the namespace goes and the rest is cut at a whole word.
- **A detached HEAD says so**, with the short hash — it is where commits get
  lost, and silence there is indistinguishable from sitting on the trunk. A
  rebase is the exception: it detaches HEAD but records the branch it set aside,
  and that branch is still where the user is, so the tab keeps its name instead
  of taking a new hash on every step.

A worktree and a submodule are followed through the `gitdir:` file to their own
HEAD, and to the shared refs their default branch lives in.

**Following the agent live makes the branch flicker.** Across the longest
sampled session, 18 worktrees in one transcript, there were 90 transitions and
89 branch segments: median life 423 s, p25 75 s, 14 under 30 seconds and two
under 5. Almost all were worktree → root → worktree, so the reader watches a
segment appear and disappear rather than one name change into another. Two
things do not go wrong — the tab's position is a decorator in front of the title
and does not move, and the rename rate is the poll interval, so nothing renames
faster than a tab already could — but one does: a branch appearing re-truncates
the activity tail at `MaxLength`, so the end of the title changes for a reason
the reader cannot see. That is the honest cost of following the agent live, which
is what was chosen over damping it.

**This source was here before, and was removed** (2d90d74). It sat at the same
confidence but filled the *activity* slot, where the terminal title outranked it
— so it spoke only for a plain shell in a repository, and there it guessed:
`fix/filter-sentry-errors-…` became `filter`, and `develop` became a word every
tab carried alike. Moving it into the context is what makes it visible beside an
agent or an editor; reading the trunk from the repository is what retires the
guess that produced the noise.

It stays out of an ssh pane. The branch is read from the directory ssh was
launched in, which says nothing about the machine on the other end, and a branch
printed beside `prod-01` reads as that machine's.

### Working directory and the fallback

The basename of the pane's directory, which is normally the project name.
`foreground_cwd` is preferred over the shell's own `cwd`, with `cwd` behind it
for the panes Herdr reports no foreground directory for. `cwd` is the pane's
shell, and a subshell leaves it behind in the directory that shell was started
from: `chezmoi cd` runs `$SHELL` in the source directory, and the pane keeps
reporting the old `cwd` for as long as the subshell lives — the tab would go on
naming a project the user has left. The foreground process follows what is
running right now, which is the point: what is running right now is what the
pane is showing.

Both of those are the snapshot's guess, and the pane a tab is named from does
better: `pane.process_info` reports each foreground process with the directory
it is itself in, deepest first, so the last entry is the pane's own foreground
process and its directory is the pane's. That is what the poll reads, and the
snapshot's pair stays behind it for the panes nothing is read for.

The difference is what an agent spawns. `foreground_cwd` is the deepest
descendant's, and an agent's descendants are its own machinery: an MCP server
started as `uv run --directory ~/gimp-mcp` is a foreground process of the pane,
so a tab holding that agent was named after the server's directory. Reading the
agent's own directory instead names the project it is working in — which is not
the shell's `cwd` either, because that shell was left behind when the user
moved on.

Directories that say nothing — the home directory, the filesystem root, a
relative path — yield nothing, and a tab left with no name at all becomes
`Shell`. On Windows the home directory is matched without regard to case, which
is how Windows spells one, and Herdr's own title for an idle pane there,
`pwsh in dashboard`, is refused as a shell prompt is: it names the shell and
the directory the context already names.

## A worktree does not say its branch twice

`git worktree add ../feat-oauth feat-oauth` makes a directory and a branch of
the same name, and the two are one fact rather than two: the title would read
`feat-oauth › feat-oauth › nvim`. The branch is dropped when it matches the
directory exactly, because the directory leads the title and the branch only
qualifies it.

The rule is about a pane whose *own* directory is a worktree — a user who moved
into one and is working there themselves. A branch that came from an agent's
directory never reaches it: the context stays the pane's, so the two halves are
read from different directories and are two facts rather than one.

Exactly, and no more than that — the rule below makes the same trade. A worktree
whose directory spells its branch differently (`xl-knp-3` against `xl-knp.3`)
keeps both, because nothing here can tell a near-miss from two real facts.

## The workspace is not repeated

Herdr shows the workspace above its tabs, so a tab in the workspace it is named
after spends half its width repeating what is already on screen. That half is
dropped: in a workspace called `dashboard`, a tab reads `nvim › auth.ts` rather
than `dashboard › nvim › auth.ts`.

It is dropped only when something else remains — a tab reduced to nothing has
lost more than it saved — and only on an exact match, so a tab whose directory
has left its workspace behind is exactly the one that keeps saying where it is.
A branch counts as something remaining, and the match is against the directory
alone, so a tab in the workspace of the repository it is in reads `feat/oauth ›
nvim`: the half that repeats goes, the half that distinguishes stays. An agent's
name counts too, but only while it is going to be shown — which is why it is
dropped before this runs rather than after.

## A pane is named for what tells it from its tab

Pane naming is on unless it is turned off ([configuration](./configuration.md)).
The same chain names a pane, from that pane's own state rather than from
the pane its tab speaks through — which is the point, because a tab speaks
through one pane and the goto panel lists them all.

The panel puts a pane's row **under** its tab's, so the rule of the section
above applies again with the tab in the workspace's place: a part the tab's
title was built from is worth less on the pane's row than a part that tells the
two apart. It is the same rule in the code too (`withoutAbove`): a title drops
what each row shown above it carries, the workspace for a tab and then the tab
for a pane, and keeps everything when that would leave nothing. A split of two
agents in one repository reads

```
pane-rename › herdr-auto-title pane      the tab, truncated
  herdr-auto-title pane 重命名            the pane the tab speaks through
  herdr-reviewr                          the pane doing something else
```

The comparison is against what the tab was built from, **not** against the tab's
finished title. Those differ whenever the tab dropped a part for repeating the
workspace: the workspace is still on screen above them both, so a pane that
picked it back up would put it there a third time. What the tab is built from is
read from the tab's own pane, so that pane is read even when the user has claimed
the tab — unread, it would have no branch, and whether a pane kept the branch
would depend on the order the panes happened to be named in.

**The activity is never dropped, whatever the tab says.** Where a pane is has a
row of its own above it; what it is doing is the whole of what a pane's row is
for. That is why the first row above keeps its activity and loses the directory
— and why it ends up carrying more than the tab, which had to truncate the same
words to fit the directory in front of them.

**Two panes of one tab are therefore asymmetric, and it reads as a bug.** Two
agents on two worktrees of one repository leave the tab named from the pane it
speaks through, so that pane's row loses the branch the tab already carries
while its sibling keeps its own:

```text
dashboard › feat/oauth › claude › Poll loop rework   the tab
  Poll loop rework                                   the pane the tab speaks through
  fix/token › Token refresh                          the pane doing something else
```

One row shows a branch and the other does not, which is the rule working rather
than failing. It only bites where the speaking pane has an activity: with
nothing but a context and a branch, everything it has is on the tab already, and
the `kept == (Parts{})` guard hands it back, so the row reads `feat/oauth ›
claude` instead of nothing.

**A pane whose only fact is its directory keeps it**, and then does repeat its
tab. There is nothing else known about such a pane, and the alternative is what
Herdr shows for a pane with no label at all: the name of the agent in it, which
is the same word on every row of the session and the reason this setting
exists.

## The agent's name is optional

`HERDR_AUTO_TITLE_AGENT_NAME=false` leaves the agent's name out of every title.
Some terminals say which agent holds a pane on their own, and a name repeated in
the tab costs columns the work could use.

Off means off, including the case where the name is the entire title: a pane
whose agent has reported nothing then reads as its directory, `dashboard`, or as
the generic fallback where there is no directory either. Keeping the name for
that one case would make the setting a rule with an exception, and the exception
would have to be explained to everyone who set it.

The name is still stripped from the *edges* of an activity that carries it,
under either value, so an agent signing its terminal title cannot smuggle the
name back in as text. Nothing looks for the name anywhere else in an activity: a
repository called `claude-mcp` keeps its title.

**It is a switch, not a format string.** A template — `{agent} › {activity}`,
with the name dropped by leaving `{agent}` out — is the more general answer, and
it costs a grammar to parse, validate and document, plus a rule for the
separator a placeholder leaves dangling when it resolves to nothing. That last
case is not hypothetical: an agent that never reports its work is ordinary, and
`{agent}: {activity}` would leave `dashboard › claude:` in the tab bar. One bit
was what was asked for, so one bit is what is stored.

Because the name is a field of its own rather than a prefix glued onto the
activity, the repetition check sees the activity as it is. A tab whose agent
titles its terminal after the project it was started in reads
`dashboard › claude` rather than `dashboard › claude › dashboard`.

## The position is not a part of the title

Every title carries the tab's place in its workspace in front of it, which is
the key that switches to that tab: `3 · dashboard › nvim`. Herdr labels an
unnamed tab with that same position, so naming a tab is what takes the number
away — and a tab bar of names is a tab bar the user has to count along to reach
the fourth one.

**It is a decorator, `Numbered` (`internal/resolver/position.go`), not a
source.** A source answers what a tab is about from what a pane holds; the
position says nothing about that and comes from the workspace instead. Wrapping
the resolver keeps the ladder about content, and keeps `Resolve` returning the
label that will actually be set — which is what the manual-rename bookkeeping
compares against.

Three things follow from what the tab bar does with a title:

- **The position leads.** Truncation cuts the tail (see
  [Sanitization](sanitization.md)), so a position at the end is the first thing
  a long title loses — exactly the titles a user is scanning when they reach
  for a key. In front it also puts every number in one column.
- **The mark is `·`, not `›`.** The parts separator would read as if the
  position were one more thing the title says about the tab.
- **It is counted against `MaxLength`, not added to it.** The decorator reads
  that bound off the resolver it wraps rather than being handed one of its own,
  so there are not two numbers to keep in step. The body is only cut to what
  the prefix leaves — it arrives sanitized, and truncating an already-truncated
  title again is the same cut, one column further in. Where nothing would be
  left — a tab bar narrower than the number itself — the number goes and the
  name stays.

`HERDR_AUTO_TITLE_POSITION=false` drops the decorator.
