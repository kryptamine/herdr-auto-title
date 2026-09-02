---
description: How to write comments, and what never to comment
---

# Comments

Go, and this repository's Go. Comments here are in English, like everything
else written into the repository, whatever language the request was made in.

## Rules

- These rules cover every kind of comment: doc comments on packages, types,
  functions, methods, fields and constants, and inline ones inside a function.
- Comment only where it is genuinely needed: a declaration or a block whose
  intent is not obvious at a glance. Obvious code gets none, exported or not —
  `NewChanges` returning a `*Changes` needs no doc comment, and a comment that
  only expands the identifier's name is worse than silence.
- **A comment is at most three lines.** A hard cap. Anything longer is
  explaining something the code cannot hold, and that explanation belongs in
  [docs/architecture](../../docs/architecture/) with one line here pointing at it.
- A doc comment starts with the name it documents and reads as a sentence:
  `// paneKind names what a pane is running, or "" when that cannot be said.`
  Inline comments inside a function need neither the name nor a full sentence.
- One package comment per package, on the file named after it.
- Explain the gist of the thing the comment sits on: what it does, plus the why
  when that is not obvious. Never who calls it or when it runs — call sites
  move and the comment then lies.
- Never record provenance: which probe, which issue, which measurement session,
  which commit. That is what `git log`, `CLAUDE.md` and `docs/architecture`
  are for. A *measured value* is worth stating; the session that measured it is
  not.
- Never justify code somewhere else. A contract is stated where it is relied
  on, never where a caller might one day lean on it.
- Keep a comment that stops a reader "fixing" the code and breaking it: a
  measured constant, a constraint the Herdr API imposes, an ordering that
  matters, a case that looks unhandled and is not.
- Plain language, plain words. If a sentence needs a second read, rewrite it
  shorter.
- Do not restate a line the code already states (`// increment the counter`
  above `counter++`).
- A `//nolint` names its linter and says why in the same line; `nolintlint`
  rejects it otherwise.
- In tests, a comment says what the case protects — the behaviour that would
  otherwise be lost — never what the assertion does.
- A field, method or constant deleted takes its comment with it.

## Examples

Bad, restates the code and names the caller:

```go
// Observe is called by App.poll on every tick.
// It stores the panes from the snapshot.
func (c *Changes) Observe(panes []herdr.PaneInfo) { ... }
```

Good, what it does and the why the signature cannot state:

```go
// Observe records a poll: panes whose revision moved changed just now and are
// no longer described by what was read of them, and panes the session no
// longer holds are forgotten.
func (c *Changes) Observe(panes []herdr.PaneInfo) { ... }
```

Bad, a rationale essay with its provenance:

```go
// This constant exists because when the poll loop started reusing process
// reads it turned out that a pane's revision does not move for every change
// in what runs there. Over a ten-minute probe of a live eight-pane session on
// Herdr 0.8.2 the foreground processes changed nine times while the revision
// moved only four, so a pane running a build kept the name of whatever it
// started with until it happened to draw, and the read now expires on its own.
const processRefresh = 2 * time.Second
```

Good, the same thing in three lines, with the paragraph left in the docs:

```go
// processRefresh is how long a process read is reused for. A pane's revision
// cannot carry this alone — measured, it moved for only four of nine process
// changes — so see docs/architecture/poll-loop.md before raising it.
const processRefresh = 2 * time.Second
```

Good, a why the code cannot state:

```go
// Herdr labels an unnamed tab with its position, and tab.rename stores an
// empty label verbatim, so both spellings mean "nobody named this tab".
if label == "" || label == position {
```

Good, none at all:

```go
func NewChanges() *Changes {
	return &Changes{
		panes: make(map[string]paneChange),
		now:   time.Now,
	}
}
```

## Comment pass

Write the code first. As the last step before reporting, check the comments
this change added or touched against the rules above. Edit comments only, never
code.
