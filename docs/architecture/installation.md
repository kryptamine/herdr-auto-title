---
type: doc
title: 'Installation'
description: 'How an install puts a binary in the plugin root: the release binaries CI attaches and attests, why one is downloaded only into a checkout of the commit it was built from, a Go build for anything else, why a release stays a draft until its binaries are attached, and why the scripts are shell and PowerShell.'
tags: [architecture, release]
created: 2026-10-24
generated: { by: claude-code/opus-5-5, at: 2026-10-27T12:00:00+03:00 }
---

# Installation

`herdr plugin install` clones the repository and runs the manifest's
`[[build]]` step in the fresh checkout. That step is `go build`, so nobody
without Go 1.24 can install the plugin. `scripts/install.sh`, or
`scripts/install.ps1` on Windows, take its place once a release carries
binaries, and with them Go is needed only for a checkout that is not a
release. Until then releases get binaries and nothing downloads them, so the
release job is proven before any install depends on it.

## What a release carries

When release-please cuts a release, the `binaries` job in
`.github/workflows/release-please.yml` runs `make dist` on the release tag and
attaches what `scripts/dist.py` builds:

```text
herdr-auto-title-linux-amd64        herdr-auto-title-windows-amd64.exe
herdr-auto-title-linux-arm64        herdr-auto-title-windows-arm64.exe
herdr-auto-title-macos-amd64        SHA256SUMS
herdr-auto-title-macos-arm64        COMMIT
```

Every binary is built with `CGO_ENABLED=0`, so it needs nothing on the machine,
musl and glibc Linux alike. `COMMIT` is the commit they were built from.
**The names are a contract with every install of that release**: renaming one
sends those installs to a Go build.

The job lives in the release-please workflow, not in one of its own on
`push: tags`, because GitHub starts no workflow for a tag pushed with
`GITHUB_TOKEN`, which is how release-please pushes it.

`SHA256SUMS` comes from the same release as the binaries, so it proves only
that a download arrived intact. Who built them is a build provenance
attestation the job records for every file it lists, checked with
`gh attestation verify <file> --repo kryptamine/herdr-auto-title`. An install
does not check it: it would need `gh` on every machine.

On every pull request CI runs `make dist` the way the job does, and the
`dist-run` jobs run each binary's `--version` on a runner of its own system
and architecture, so a cross-built target that does not start fails there.

## What an install does

1. Read the version from the checkout's `herdr-plugin.toml` and fetch that
   release's `COMMIT` from `releases/download/v<version>/`.
2. **If it equals `git rev-parse HEAD`**, the checkout is that release.
   Download the binary for this platform, check it against `SHA256SUMS`, run
   it from the plugin root and keep it only if `--version` reports the
   manifest's version.
3. **Otherwise, or if step 2 failed**, build with Go. A checkout that is not
   the release, or whose release `COMMIT` could not be fetched, says so first,
   and names `--ref stable` as the way to the latest release without Go.
   Where no binary could be downloaded at all, on a platform with none
   published or a Linux or macOS without `curl` or `wget`, it gives no hint.
4. Without Go the install fails, with the same hint when the checkout was not
   a release.

There is no fallback to another release, such as `releases/latest/download/`.

### Why by commit and not by version

A release binary is only right next to the tree it was built from. The tree
carries the manifest's `[[startup]]` and actions and `config.env.example`; a
binary from another commit may not take the arguments the manifest passes, or
silently ignore the settings the example documents.

Matching by version cannot tell them apart. A checkout of `main` is usually
ahead of the latest release, yet its `herdr-plugin.toml` still names that
release, because release-please changes the version only in the release
commit. A plain install would pair `main`'s tree with the last release's
binary.

Herdr's checkout keeps `.git`, so `git rev-parse HEAD` is the installed commit,
but it holds no tags: Herdr 0.9.5 fetches the one ref at depth 1 and checks it
out detached. So the release's commit has to come from the release itself.
Every install that gets a binary is therefore a release tag or `stable`, a
branch CI moves to each release only after its binaries are attached. A plain
install gets `main` and needs Go, which is what the hint is for.

The `--version` check catches a release whose assets do not belong to it.
What `--version` prints comes from `cmd/herdr-auto-title/version.go`, which
release-please updates with the manifest, and a test keeps the two equal.

### Updating

`herdr plugin update` is an install again. On Herdr 0.9.5 it fetches the ref
the plugin was installed with, or `origin HEAD` without one, into a new
checkout; it stops there as up to date when that is the installed commit, and
otherwise runs `[[build]]` in it. An install that follows `stable` therefore
updates to the next release's binary, and one pinned to a tag never moves.
Like an install, an update starts nothing: the running instance stays until
the `restart` action or the next server start.

### Why releases start as drafts

Between release-please tagging a release and the `binaries` job attaching its
files there are a minute or two in which the release has no binaries. So
release-please creates the release as a draft (`"draft": true`, with
`"force-tag-creation": true` so the tag exists at once and the next changelog
finds it), and the job publishes it only after the upload. Watchers are told
of a release only once it can be installed, and `stable` moves only after
that.

An install of the tag itself in that window finds no `COMMIT` and builds from
source, or fails without Go.

If the `binaries` job fails, the release stays a draft. Fix the cause and
re-run the failed job: it checks out the tag again and publishes.

## Why shell and PowerShell

Everything else in `scripts/` is Python, but the install step runs on
machines that have neither Go nor Python: Windows ships no Python, and macOS
has one only once the command line tools are installed. `sh` with `curl` or
`wget` and `sha256sum` or `shasum` is on every Linux and macOS, and Windows
PowerShell 5.1 with `Invoke-WebRequest` and .NET on every Windows 10 and 11.
`git` is there because Herdr needs it to clone. The hash is .NET's rather than
`Get-FileHash`: started from PowerShell 7, Windows PowerShell inherits its
module path and cannot load that cmdlet.

The two scripts do the same steps in the same order, and
`scripts/test_install.py` runs each against releases served locally, in CI on
all three systems.

## Where the binary goes

Where `go build` put it: the plugin root, as `herdr-auto-title` or
`herdr-auto-title.exe`, so `[[startup]]` and the actions are unchanged. The
step runs in a checkout Herdr makes anew for each install and update
(`plugins/github-installations/<plugin>-<hash>/<pid>-<n>/checkout` on Herdr
0.9.5), so no running instance holds that file. A download is still staged
beside it and moved over it in one rename, and run there for its `--version`
because `$TMPDIR` may be mounted `noexec`.
