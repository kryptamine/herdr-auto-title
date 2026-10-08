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

Steps 1, 4 and 5 are regular-expression passes, and each is guarded by a plain
check for the one character it needs: an escape, a doubled space, a separator.
None of them usually matches, and unguarded their backtracking measured half of
what naming a tab costs — 13.9 µs a tab against 6.9 µs with the guards, and 106
allocations against 24. Each guard is exact rather than approximate: every
sequence `ansiRe` matches opens with an escape; step 2 has already turned every
kind of space into a plain one, so a run left to collapse is two of those; and
neither separator pass can match a value carrying no separator.

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

  A path can hold spaces — PowerShell 7 titles its window
  `C:\Program Files\PowerShell\7\pwsh.exe` — and a title is split into words
  before anything knows where a path ends. Three rules say how far the words
  after a location belong to it, the first that applies deciding:

  1. **A path that opens a bracket or a quote runs to the word that closes
     it**: `auth.ts (C:\Users\Jane Doe) - Nvim` keeps `auth.ts - Nvim`, and a
     command `"C:\Program Files\nodejs\node.exe" server.js` keeps `server.js`.
     The delimiter says where the path ends, so this one holds on every
     platform and at the start of a title too: `[C:\work\app] build failed`
     keeps `build failed`. A program quoted there is the exception and goes
     on by the second rule, so `"C:\Program Files\Git\bin\bash.exe" --login`
     keeps nothing, as the same path unquoted would. Punctuation standing
     alone before the closer means nothing was opened:
     `sort (/tmp/in.txt | uniq` keeps `sort | uniq`. A `<` is no bracket,
     since before a path it redirects: `make </dev/null && ./run` keeps
     `make && ./run`.
  2. **A title that opens with a Windows path is that path up to the first
     punctuation standing alone**: `C:\Users\Jane Doe - Fix login` keeps
     `Fix login`, and `C:\Users\Jane Doe` alone keeps nothing. The cost is a
     command written straight after the path, `C:\work\app git status`, which
     goes with it — a trade, since nothing in the text tells `Doe` from
     `git status`. PowerShell and cmd both title a pane with the program's path,
     and cmd puts ` - ` before a command it runs, so the cost falls on a title
     neither sets, or on a prompt that opens with a folder or a program.
  3. **Anywhere else a word belongs to it when it carries the path on**,
     holding a backslash past its first character, and a word without one
     only when the next word does: `Program Files (x86)\Steam`. It never runs
     past punctuation standing alone or another location:
     `C:\WINDOWS\system32\cmd.exe - python scripts/probe.py` keeps its
     command, `Copy C:\src to D:\dst` keeps `Copy to`, and
     `cd C:\work\app & python scripts\probe.py` keeps
     `cd & python scripts\probe.py`. A backslash opening a word is an escape,
     so `fix C:\work\app where \d+ fails` keeps `fix where \d+ fails`.

  The last two also end a path at a word closing a clause or a prompt:
  `C:\work\app: command not found` keeps `command not found`, and
  `C:\Users\Jane Doe> npm test` keeps `npm test`. They end one at a file too,
  a last word with an extension:
  `fix C:\work\auth.ts where it fails` keeps `fix where it fails`, and a
  prompt `C:\Users\jane\Desktop\error.png what is wrong` keeps its question.
  A program opening a title is the exception, since what follows is arguments.

  A share is rooted the Windows way only as `\\host\share`, with both parts:
  `\\n` and `\\r\\n` are escaped backslashes in a sentence.

  Windows puts a dash inside the names of the folders it syncs: a work
  OneDrive is `OneDrive - Contoso`, a SharePoint library
  `Contoso\Marketing - Documents`. A dash standing alone therefore stays in a
  Windows path when it follows a folder named `OneDrive`, or when the word
  after it carries the path on, so `plan.docx (C:\Users\jane\OneDrive -
  Contoso\Documents) - Nvim` keeps `plan.docx - Nvim`. After a file it still
  separates: `C:\WINDOWS\system32\cmd.exe - scripts\build.bat` keeps its
  command.

  Only the first rule reads a path not rooted the Windows way. Such a path
  holds a backslash only to escape a space, so a word ending in one carries it
  on: `vim /tmp/My\ Big\ Folder/x.txt` keeps `vim`. What remains is a spaced
  name at the end of a path inside a sentence: `editing C:\Users\Jane Doe`
  keeps `editing Doe`, and the reverse, a relative path straight after a
  folder: `git diff C:\work\app src\auth.ts` keeps `git diff`. Looking the
  path up on disk would settle both, and is ruled out: the title is
  untrusted, and on Windows a lookup of `\\host\share` connects to that host
  and offers it the user's credentials.
- **Program names.** A value that only names a program or a shell — `zsh`,
  `node`, `Claude Code`, `Agent` — says what is running, which is a *kind*, not
  what the user is doing. `pwsh.exe` is the same name as `pwsh`, since a Windows
  title carries the extension. An agent echoing its own name is caught
  separately, by comparison against the agent Herdr recognized in the pane
  rather than against the table, so it works for agents nobody listed.
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
