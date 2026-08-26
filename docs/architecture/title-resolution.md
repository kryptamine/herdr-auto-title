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

Structurally a title is three fields, `Parts{Context, Branch, Activity}`
(`internal/resolver/resolver.go`): *where* the user is and *what* they are
doing. The branch belongs to the first of those — it qualifies the directory
rather than standing beside it as a separate kind of thing.

## One pane speaks for the tab

A tab holding several panes is named after one of them, never after a blend of
both. `SelectContextPane` (`internal/state/tab.go`) picks, in order:

1. the focused pane;
2. failing that, a pane running an agent that is `working` or `blocked` — a
   split where the user left an agent running is about that agent, even though
   the pane below it saw the last update;
3. failing that, the pane that changed most recently.

Ties break on the most recent change and then on pane ID, so identical state
always yields the same choice. Both halves of the name then come from that pane
alone.

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
`dashboard › claude › Implement OAuth scopes` after.

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
`dashboard › nvim` for an editor with no file open.

A mapping from command lines to friendlier names (`yarn dev` → `Dev`) was
specified and is deliberately not built: the commands it would map are invisible
in the process table, visible only in the terminal title, and a source below the
terminal title can never fill an activity the terminal title has already filled.

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

### The git branch

The branch checked out in the pane's directory, read from the files under
`.git` and never by running git: `git rev-parse` measured 12.37 ms against
0.019 ms for reading `HEAD`, on a poll whose whole snapshot costs 0.47 ms. The
reading is not cached — at 0.038 ms including the walk up to the repository, a
fresh answer costs less than remembering a stale one, and a checkout shows up in
the tab within one poll.

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
HEAD, and to the shared refs their default branch lives in. Agents run in
worktrees, and a worktree's directory is exactly the kind that names nothing.

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

The basename of the pane's directory, which is normally the project name. The
shell's own `cwd` is preferred over `foreground_cwd`, which follows whatever is
running right now. Directories that say nothing — the home directory, the
filesystem root, a relative path — yield nothing, and a tab left with no name at
all becomes `Shell`.

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
nvim`: the half that repeats goes, the half that distinguishes stays.

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
  so there are not two numbers to keep in step. The body is re-sanitized to
  what the prefix leaves, and truncating an already-truncated title again is
  the same cut, one column further in. Where nothing would be left — a tab bar
  narrower than the number itself — the number goes and the name stays.

`HERDR_AUTO_TITLE_POSITION=false` drops the decorator.
