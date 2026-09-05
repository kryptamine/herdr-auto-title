# Contributing to Herdr Auto Title

What the plugin does and how to install it are in the
[README](README.md). This is how to work on it.

## Before you start

- Open an issue first for anything non-trivial, and settle the scope there
  before you spend an evening on it.
- Small fixes go straight to a PR: a typo, an obvious bug, a dependency bump.
- If an issue rests on something false about Herdr's API, say so on the issue
  and correct it rather than working around it in silence.
- Anything with security consequences goes to the address in
  [SECURITY.md](SECURITY.md), not to an issue.

## Getting set up

You need what the README asks for to install the plugin, plus a checkout:

```sh
git clone https://github.com/kryptamine/herdr-auto-title.git
cd herdr-auto-title
make check      # fmt + vet + lint + test; the gate before any commit
make run        # build and run it in your current session
```

> [!WARNING]
> Development runs against your **real** Herdr session, so your own tabs get
> renamed while you work. Run the plugin in the foreground, never in the
> background, and check `make ps` when something behaves oddly — `make stop`
> ends the instances you forgot.

`make` on its own lists every target. The full workflow is in
[docs/development.md](docs/development.md), and how the plugin works and why is
in [docs/architecture](docs/architecture/).

## Working on a change

- Branch from `main`.
- **Everything written into this repository is in English**: comments, commit
  messages, log and error text, documentation, test names.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org):
  `<type>(<scope>): <subject>`, imperative and lowercase, no trailing period,
  72 characters at most. Types in use are `feat`, `fix`, `docs`, `test`,
  `refactor`, `perf` and `chore`; the scope is a package or area (`resolver`,
  `state`, `herdr`, `app`). The body explains **why** — the diff already says
  what. One logical change per commit, and never a co-author trailer.
- Tests land with the behaviour they cover. `go test -race` is the gate, not
  `go test`: the poll loop and the change history it keeps are exercised
  concurrently.
- **A struct field exists only if code reads it.** Herdr's wire objects carry
  far more than Auto Title needs, and an unread field is a promise to keep
  something working that nothing exercises.
- A comment is at most three lines, and it explains what is surprising rather
  than what is visible. A decision that needs a paragraph goes into
  `docs/architecture/` with one line in the code pointing at it.
- Everything in `scripts/` is Python 3 and uses the standard library only. Shell
  stays in the Makefile's one-line recipes.
- Never pass a terminal-derived value to a shell. Renames go over the socket
  API.
- Probe before assuming anything about Herdr's API (`make probe-*`,
  `scripts/probe.py`): the originating specification is wrong in several
  places. Record what a probe teaches you in
  [docs/architecture/herdr-socket-api.md](docs/architecture/herdr-socket-api.md),
  which is the record; `AGENTS.md` keeps only the facts that mislead in silence.

## Pull requests

- `make check` green locally. CI then runs `go vet`, `go build` and
  `go test -race` on Go 1.24 and stable, the formatters and `golangci-lint`
  once, and a build and test run on macOS.
- **PRs are merged by rebase.** Merge commits are disabled on the repository:
  GitHub puts the PR title into the merge commit, and since the titles here are
  conventional, release-please counted every change twice. Squashing would
  collapse a PR into one changelog line and lose the per-commit granularity that
  "one logical change per commit" exists to produce.
- Versions, tags and `CHANGELOG.md` belong to release-please, which also bumps
  the version in `herdr-plugin.toml`. Do not edit any of them by hand.
- Keep a PR to one thing. A refactor, a feature and a formatting sweep are three
  pull requests.

## Code of conduct

Be kind, assume good faith, keep it about the work.

## License

By contributing you agree that your contribution is licensed under the
[MIT License](LICENSE).
