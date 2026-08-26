---
type: doc
title: 'Sanitizing Untrusted Values'
description: 'Every value that reaches a tab title comes from terminal output and is treated as hostile: what is stripped, what is rejected as saying nothing, and why the length limit is counted in terminal columns rather than characters.'
tags: [architecture, security]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Sanitizing Untrusted Values

Every candidate for a title — a directory name, a terminal title, an agent
title, an ssh destination, a topic read out of an agent's own transcript —
originates in terminal output, in a file an agent wrote, or in a path someone
chose. All of it is treated as untrusted input.

The one rule that is never bent: **nothing derived from terminal output is
passed to a shell.** Renames go over the socket API, and Auto Title runs no
subprocess at all — there is no `sh -c` anywhere to pass anything to.

## Cleaning

`Sanitize` (`internal/resolver/sanitize.go`) is the single gate. In order, it:

1. strips ANSI escapes — CSI sequences, OSC strings and single-character escapes;
2. maps every kind of space to a plain one — the non-breaking and the
   ideographic included — and drops every control character;
3. drops format characters, which are invisible and forge what the reader sees:
   a right-to-left override reverses the label, a zero-width space makes a
   second tab read identically to the first, a bidi isolate reorders what
   surrounds it. The zero-width joiner is the one exception, kept because
   emoji clusters are built out of it;
4. collapses runs of whitespace;
5. collapses runs of the separator and normalizes the spacing around it, so a
   value that already contains `›` cannot forge extra structure;
6. trims leading and trailing separators and whitespace;
7. truncates to the limit.

`Sanitize` is idempotent: running it on its own output changes nothing.

## Rejecting values that say nothing

Cleaning is not enough — a value can be perfectly well-formed and still be
worthless as a title. `Meaningful` and the tables in
`internal/resolver/generic.go` reject three kinds:

- **Locations.** Absolute paths, home-anchored paths and URIs are removed,
  because the context half already says where the user is and a path is long
  enough to push the useful part past the length limit. An editor titling its
  window `auth.ts (~/work/dashboard/src) - Nvim` contributes `auth.ts - Nvim`; a
  shell titling it `~` contributes nothing. **Relative paths survive**, so
  `Fix bug in src/auth.ts` stays intact.
- **Program names.** A value that only names a program or a shell — `zsh`,
  `node`, `Claude Code`, `Agent` — says what is running, which is a *kind*, not
  what the user is doing. An agent echoing its own name is caught separately, by
  comparison against the agent Herdr recognized in the pane rather than against
  the table, so it works for agents nobody listed.
- **Shell prompts.** `root@psi:`, `alex@macbook:~/work`. A prompt says who and
  where, which the context has already said, and never says what the user is
  doing.

A source whose value does not survive declines, and the resolver falls through
to the next rung rather than producing a junk name.

## The limit is columns, not characters

A limit on a title is a limit on the room it takes in the tab bar, and a rune
count is not that.

- **Width.** CJK characters and emoji occupy two columns each, so a
  sixty-four-rune cap let a Japanese title fill a hundred and twenty-eight
  columns.
- **Grapheme clusters.** Several code points often make the one character a
  reader sees: a family emoji is four joined by zero-width joiners, a flag is two
  regional indicators, a thumb carries its skin tone as a second code point.
  Cutting by rune cut inside them, and `work 👨‍👩‍👧‍👦` came back as half a
  family ending on an invisible joiner.

`splitAtWidth` walks grapheme clusters and sums their widths
(`github.com/rivo/uniseg`, the project's only dependency), so a cluster is kept
whole or left out. `HERDR_AUTO_TITLE_MAX_LENGTH` is therefore counted in
columns; for ASCII the two units are the same number, which is why the default
did not change when this did.

Truncation also leaves no dangling separator behind: a title cut mid-structure
says a part was lost without saying which, so `abcdefg › hij` cut at nine
columns is `abcdefg`, not `abcdefg ›`.
