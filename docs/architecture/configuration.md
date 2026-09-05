---
type: doc
title: 'Configuration'
description: 'Why Auto Title reads a configuration file at all, where that file lives and why it is not the directory Herdr offers, why the environment beats the file, why the parsing is a library rather than ten lines of our own, and why nothing is reread while the plugin runs.'
tags: [architecture]
created: 2026-08-26
generated: { by: claude-code/opus-5, at: 2026-08-26T14:14:17+03:00 }
---

# Configuration

Every setting Auto Title has is one of eight `HERDR_AUTO_TITLE_*` variables,
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

`config.env`, in the directory that already holds the manual-rename locks:

| Platform | Path |
|----------|------|
| macOS | `~/Library/Application Support/herdr-auto-title/config.env` |
| Linux | `~/.config/herdr-auto-title/config.env` (`$XDG_CONFIG_HOME` if set) |

That is `os.UserConfigDir()`, the same call `state.DefaultManualPath` makes.
One directory holds everything Auto Title owns, and a user who has found one
file has found the other. The path is fixed: no variable and no flag moves it,
because a configuration file whose location is itself configurable needs a
configuration file to find it.

**Herdr offers a directory of its own and Auto Title does not use it.** Herdr
creates `~/.config/herdr/plugins/config/<plugin id>/` and prints it in `herdr
plugin list`, and it names `HERDR_PLUGIN_CONFIG_DIR` among the variables it
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

## Why it is not reread

The file is read once. Half the settings are consumed in `main.run` while it
builds the resolver chain — `MAX_LENGTH` and `BRANCH_MAX` are baked into
`resolver.Default`, `POSITION` decides whether the chain is wrapped at all — so
rereading the file mid-run would apply some settings and quietly ignore others.
An honest restart is better than a reload that works half the time, and the
plugin restarts in the time it takes the server to start it again: `herdr server
stop`, then `herdr`.
