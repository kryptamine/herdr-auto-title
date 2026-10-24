---
type: doc
title: 'Installation'
description: 'How an install puts a binary in the plugin root without Go: the release binaries CI attaches, the order the install scripts try them and a source build in, why a release stays a draft until its binaries are attached, and why the scripts are shell and PowerShell.'
tags: [architecture, release]
created: 2026-10-24
generated: { by: claude-code/opus-5-5, at: 2026-10-24T19:50:00+03:00 }
---

# Installation

`herdr plugin install` clones the repository and runs the manifest's
`[[build]]` step in the fresh checkout. That step used to be `go build`, so
nobody without Go 1.24 could install the plugin. It is now
`scripts/install.sh`, or `scripts/install.ps1` on Windows, and Go is needed
only when no release binary fits.

## What a release carries

When release-please cuts a release, the `binaries` job in
`.github/workflows/release-please.yml` runs `make dist` on the release tag and
attaches what `scripts/dist.py` builds:

```
herdr-auto-title-linux-amd64        herdr-auto-title-windows-amd64.exe
herdr-auto-title-linux-arm64        herdr-auto-title-windows-arm64.exe
herdr-auto-title-macos-amd64        SHA256SUMS
herdr-auto-title-macos-arm64
```

Every binary is built with `CGO_ENABLED=0`, so it needs nothing on the machine,
musl and glibc Linux alike. **The names are a contract with every install of
that release**: renaming one makes those installs fall back to Go.

The job lives in the release-please workflow, not in one of its own on
`push: tags`, because GitHub starts no workflow for a tag pushed with
`GITHUB_TOKEN`, which is how release-please pushes it.

## The order an install tries

1. **This version's release binary.** The version is the one in the
   checkout's `herdr-plugin.toml`. The binary is downloaded from
   `releases/download/v<version>/`, checked against `SHA256SUMS`, and kept
   once it runs `--version` from the plugin root.
2. **A Go build**, exactly what the step used to be.
3. **The latest published release**, from `releases/latest/download/`, checked
   the same way, with a message naming the release it installed instead.
4. Otherwise the install fails, saying whether Go is missing or its build
   failed.

The version a binary reports is not compared with anything. It comes from
the same release as the `SHA256SUMS` it matched, so the checksum already says
which release it is; running it says only that it works on this machine.

`HERDR_AUTO_TITLE_BUILD=source` in the environment of `herdr plugin install`
skips straight to the Go build and fails if it does.

### Why by version and not by commit

A checkout of `main` is usually ahead of the latest release, but its
`herdr-plugin.toml` still names that release, because release-please changes
the version only in the release commit. Matching by version therefore gives
an install without `--ref` the latest release and one with `--ref vX.Y.Z` that
release, which is what the README promises. Matching by commit would send
everyone installing from `main` to a Go build and make the binaries useless
for the plain install most people run. Unreleased code is one setting away:
`HERDR_AUTO_TITLE_BUILD=source`.

What `--version` prints comes from `cmd/herdr-auto-title/version.go`, which
release-please updates with the manifest, and a test keeps the two equal.

### Why the latest release comes last, and why releases start as drafts

Between the release commit landing on `main` and its binaries being attached
there are a minute or two in which step 1 finds nothing. A Go build is the
better answer when Go is there, since it is this version. Without Go, the
previous release is better than a failed install.

`releases/latest/download/` resolves to the latest *published* release. If a
release were published before its binaries, that would be the new, empty one
and step 3 would find nothing either. So release-please creates the release as
a draft (`"draft": true`, with `"force-tag-creation": true` so the tag exists
at once and the next changelog finds it), and the `binaries` job publishes it
only after the upload. The window is then covered by the release before.

If the `binaries` job fails, the release stays a draft and installs keep
getting the previous one. Fix the cause and re-run the failed job: it checks
out the tag again and publishes.

## Why shell and PowerShell

Everything else in `scripts/` is Python, but the install step runs on
machines that have neither Go nor Python: Windows ships no Python, and macOS
has one only once the command line tools are installed. `sh` with `curl` or
`wget` and `sha256sum` or `shasum` is on every Linux and macOS, and Windows
PowerShell 5.1 with `Invoke-WebRequest` and .NET on every Windows 10 and 11.
The hash is .NET's rather than `Get-FileHash`: started from PowerShell 7,
Windows PowerShell inherits its module path and cannot load that cmdlet.

The two scripts do the same steps in the same order, and
`scripts/test_install.py` runs each against releases served locally, in CI on
all three systems.

## Where the binary goes

Where `go build` put it: the plugin root, as `herdr-auto-title` or
`herdr-auto-title.exe`, so `[[startup]]` and the actions are unchanged. The
step runs in a fresh checkout (measured on Herdr 0.9.5 as
`plugins/.tmp-install-<pid>-<ms>/checkout`), so no running instance holds that
file. A download is still staged beside it and moved over it in one rename,
and run there for its `--version` because `$TMPDIR` may be mounted `noexec`.
