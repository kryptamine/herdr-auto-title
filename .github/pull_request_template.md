## What this changes

<!--
One paragraph. The diff already says what; say why the change is the one
worth making. If it settles something about Herdr's API, say what was
probed.
-->

Closes #

## How it was checked

<!--
`make check` is the gate, but say what you looked at beyond it: a live
session, a pane arranged to reproduce the bug, a probe under `scripts/`.
-->

## Before merging

- [ ] `make check` is green locally.
- [ ] One logical change per commit, Conventional Commits, no co-author
      trailer. **PRs are rebased, never squashed or merged**, so the commit
      messages are what lands in the history and in the changelog.
- [ ] Tests land with the behaviour they cover.
- [ ] `CHANGELOG.md`, the tags and the version in `herdr-plugin.toml` are
      untouched — they belong to release-please.
- [ ] A probe that taught something new is recorded in `CLAUDE.md` and in
      `docs/architecture/herdr-socket-api.md`.
- [ ] This pull request is one thing.
