---
name: validate-agent-work
description: Use when finishing any change to this repository, or when the user says "validate", "check my work", "run checks", or "verify changes". Also use automatically after adding a feature, fixing a bug, or editing Go, Python or documentation here.
---

# Validate Agent Work

The rules live in [CLAUDE.md](../../../CLAUDE.md) and
[CONTRIBUTING.md](../../../CONTRIBUTING.md); this is the order to check them in
before handing work back.

## Steps

1. **See what changed** — the branch against `main`, and whatever is not
   committed yet:

   ```bash
   git diff --name-only main...HEAD
   git status --short
   ```

   Stage nothing: every check below runs over the whole module, and the index
   is the user's to decide.

2. **Run the gate:**

   ```bash
   make check
   ```

   `fmt` runs first and rewrites files, so re-read one before editing it again.

3. **Comment pass** — the `## Comment pass` in
   [`.claude/rules/comments.md`](../../rules/comments.md), over the comments
   this change added or touched.

4. **Read the diff against the mandatory rules in CLAUDE.md** — the language,
   commit, type, script and comment rules, and the "Working here" list. The
   linter checks none of them. Two need a search rather than a read: a field,
   method or constant added without a reader in the same change, and a probe
   whose finding has not reached `docs/architecture/herdr-socket-api.md`, which
   is the record — `CLAUDE.md` takes it only if it misleads the code in
   silence.

5. **Check nothing was left running:**

   ```bash
   make ps
   ```

6. **Fix and re-run** — fix what a step found, then run that step again to
   confirm it. If a failure is not yours to fix, say so rather than working
   around it.

7. **Report** — always say what happened:

   - Clean: a brief confirmation.
   - Found and fixed, one line each:

     ```
     Validation found and fixed 3 issues:
     - golangci-lint: unused parameter in resolver.qualify
     - Comments: dropped a caller note in app/reads.go
     - Type rule: removed PaneInfo.Cols, which nothing read
     ```

   - Still broken: what is wrong and why you could not fix it.

`make check` is the whole gate on this machine, but CI also builds and tests on
the Go floor `go.mod` names and on macOS — see CONTRIBUTING's "Pull requests".
The user should never be surprised by a failure that could have been caught
here.
