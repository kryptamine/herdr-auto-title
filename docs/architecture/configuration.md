---
type: doc
title: 'Configuration'
description: 'Why Auto Title reads a configuration file at all, where that file lives and why it is not the directory Herdr offers, why the environment beats the file, why the parsing is a library rather than ten lines of our own, and why nothing is reread while the plugin runs.'
tags: [architecture]
created: 2026-08-26
generated: { by: claude-code/opus-5, at: 2026-08-26T14:14:17+03:00 }
---

# Configuration

Every setting Auto Title has is one of ten `HERDR_AUTO_TITLE_*` variables,
read in `internal/app/config.go`. They can be set in the environment, or written
into a file that is loaded into the environment before anything reads it.

## Why a file exists

Auto Title is started by the Herdr **server**, through the `[[startup]]` entry
in `herdr-plugin.toml`. That process inherits the server's environment — not the
shell you were in when you exported anything, and not the shell of the pane you
happen to be looking at. Exporting a variable from your shell profile only
reaches the plugin if the server itself was started after that profile ran, and
it stops reaching it the moment the server is restarted from somewhere else.

So the variables were, in practice, unsettable. The file is not a second way to
configure the plugin; it is the only reliable delivery for the settings that
already existed. It is read once, at startup, by `readConfigFile`, which loads
it into the process environment; `fromEnv` then reads the environment exactly as
it did before.

## Where the file lives

`herdr-auto-title/config.env`, in the first of these directories that already
holds it — `ownPath` in `internal/app/paths.go`, which every file Auto Title
keeps is looked up through:

| Order | Directory |
|-------|-----------|
| 1 | `$XDG_CONFIG_HOME`, when it is set to an absolute path |
| 2 | `~/.config`, which is `%USERPROFILE%\.config` on Windows |
| 3 | `os.UserConfigDir()`: `~/Library/Application Support` on macOS, `%APPDATA%` on Windows |

**It was the third alone**, which made one setting two files. `~/.config` is
what a dotfiles repository syncs; `~/Library/Application Support` is macOS-only
and cannot go in one, so a user with a Mac and a Linux box kept the file twice
and could version only one copy of it. Reading `XDG_CONFIG_HOME` first is one
rule rather than a table per platform, and a user who set it has already said
where configuration goes. A relative value is ignored, as the specification
says: honoured, it would resolve against whatever directory the Herdr server
was started in.

`~/.config` is looked in on Windows too, as `%USERPROFILE%\.config`, which is
where cross-platform tools — git, mise, scoop, wezterm, opencode — already keep
their configuration, and what a dotfiles repository serving Windows syncs.
Herdr's own files being under `%APPDATA%\herdr` is Herdr's choice for its own
directory; it does not decide where a user's dotfiles live.

**The third entry is a fallback, not a legacy to migrate.** An install made
before this ordering existed keeps reading the file where it is; nothing moves
it, because moving a user's file to fix a preference they never stated is worse
than looking in two places. Only a directory that already holds the file wins,
so putting one in `~/.config` is how a user opts in, and deleting it is how they
go back.

**Each file is looked for on its own.** `config.env` and `manual-names.json`
resolve separately, so a user who copies only the configuration into a dotfiles
repository keeps the locks where they already are — which is right: locks are
machine state, not configuration, and a user syncing dotfiles is not asking for
a Mac's tab locks on a Linux box.

**A file none of them holds is created in the last one**, `os.UserConfigDir()`,
not the first. Only `manual-names.json` is ever created, and for the same reason
a fresh install must not put it in `~/.config`: that would start the directory
a dotfiles repository syncs with a file that is machine state, and a user who
later adopts `~/.config` for dotfiles would find their locks already in it. On
Linux the last entry is `$XDG_CONFIG_HOME` or `~/.config` anyway.

The list itself is fixed: no variable and no flag adds a directory, because a
configuration file whose location is itself configurable needs a configuration
file to find it. `HERDR_AUTO_TITLE_MANUAL_FILE` moves the locks, but it is a
setting read *from* `config.env`, not a way to find it.

**Herdr offers a directory of its own and Auto Title does not use it.** Herdr
creates `~/.config/herdr/plugins/config/<plugin id>/` (under `%APPDATA%\herdr`
on Windows) and prints it in `herdr plugin list`, and it names
`HERDR_PLUGIN_CONFIG_DIR` among the variables it
passes a plugin it starts — see [the socket API note](./herdr-socket-api.md) for
how far that second half is verified. Two reasons against it: it exists only when the server starts the
plugin, so a plugin run by hand — `make run`, `make dev`, every debugging
session — would look somewhere else than the same plugin run normally, and it
would split Auto Title's own files across two directories for no gain. The
README says so out loud, because `herdr plugin list` will keep suggesting
otherwise.

## Why the environment still wins

A variable already in the environment is left as it is; the file only fills what
the environment does not say. `godotenv.Load` works that way by contract, so
this costs no code.

The file is where a setting lives permanently; the environment is how one run
overrides it. That is what `make run` does — `HERDR_AUTO_TITLE_DEBUG=1` in front
of the binary — and what anyone debugging does without thinking about it. Had
the file won, a debug flag on the command line would have been silently ignored,
which is the worst behaviour available.

## Why a library

`github.com/joho/godotenv` v1.5.1: about 500 lines, MIT, and **no transitive
dependencies** — it is the plugin's second dependency and adds nothing below
itself. Auto Title's own needs are narrower than what it does, so the syntax the
file accepts is the library's, not a specification of ours:

- `KEY=value`, `#` comments, blank lines, and `export KEY=value`.
- Surrounding quotes are stripped, and `\n` inside double quotes is a newline.
- `${VAR}` and `$VAR` are expanded **from the file itself, never from the
  environment** (`expandVariables` is handed the map parsed from that file).
  `MANUAL_FILE=${HOME}/names.json` therefore yields `/names.json`. This is the
  one trap in the format, so the README warns about it.
- **A single bad line costs the whole file.** The parser returns an error and no
  values, so there is nothing to salvage line by line: Auto Title warns, naming
  the file and what the parser objected to, and every setting keeps its default.

Not warning about a key that is not ours is deliberate: `godotenv` puts every
pair it reads into the environment, and a key Auto Title does not read simply
has no effect. Nothing checks names against a list, so a typo is silent — the
cost of that is one line in the README table.

## Why naming panes is on by default, and still a setting

Auto Title names panes as well as tabs unless `HERDR_AUTO_TITLE_PANES=false`.
It was first built switched off, for two reasons, and neither held up.

**It changes what an existing user sees.** A pane nobody has labelled is listed
by the agent running in it, so a session of Claude Code panes is a column of
`claude`. That column is the problem the feature exists for rather than a
display anyone relies on, so the change shipped as a breaking one instead of as
an opt-in. The one real loss is on the first start: nothing marks a pane label
as the user's before Auto Title has watched it, so a pane labelled by hand
before then is renamed once. After that it is protected like a tab
([manual rename protection](./manual-rename-protection.md)).

**It multiplies what a poll spends.** Naming a tab reads one pane of it, so the
cost is a `pane.process_info` per tab; naming panes reads every pane. A read is
reused while its pane sits still and for at most `processRefresh`, so a pane
costs between one read every two seconds and two a second. At the measured
0.17 ms a read ([the socket API note](./herdr-socket-api.md)), twenty panes all
busy at once cost Herdr about 7 ms a second — it scales with how the user
splits, but from a floor too low to decide a default by.

The setting decides one thing only: whether `App` holds a pane resolver at all.
Everything below that — which pane is read, how it is named, whether the user
has claimed it — is the same code either way.

## Why it is not reread

The file is read once. Half the settings are consumed in `main.run` while it
builds the resolver chain — `MAX_LENGTH` and `BRANCH_MAX` are baked into
`resolver.Default`, `POSITION` decides whether the chain is wrapped at all — so
rereading the file mid-run would apply some settings and quietly ignore others.
An honest restart is better than a reload that works half the time, and the
plugin restarts in the time it takes the server to start it again: `herdr server
stop`, then `herdr`.
