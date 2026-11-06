<div align="center">
  <p>
    <img src="assets/herdr-auto-title-banner.png" alt="Herdr Auto Title" width="800">
  </p>
  <p>
    <a href="https://github.com/kryptamine/herdr-auto-title/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/kryptamine/herdr-auto-title/ci.yml?branch=main&style=for-the-badge&logo=githubactions&logoColor=white&label=CI&labelColor=000000" alt="CI status"></a>
    <a href="https://github.com/kryptamine/herdr-auto-title/releases"><img src="https://img.shields.io/github/v/release/kryptamine/herdr-auto-title?style=for-the-badge&logo=github&logoColor=white&color=0797ff&labelColor=000000" alt="Latest release"></a>
    <a href="https://go.dev"><img src="https://img.shields.io/github/go-mod/go-version/kryptamine/herdr-auto-title?style=for-the-badge&logo=go&logoColor=white&color=0797ff&labelColor=000000" alt="Go version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-0797ff?style=for-the-badge&labelColor=000000" alt="MIT licence"></a>
  </p>
</div>

https://github.com/user-attachments/assets/46fa98e7-416f-4cca-b024-82bc641cd0d2

A [Herdr](https://herdr.dev) plugin that names your tabs and panes based on
their context.

## Install

You need Herdr 0.8.2+ on macOS, Linux or Windows.

```sh
herdr plugin install kryptamine/herdr-auto-title --ref stable
herdr plugin action invoke herdr.auto-title.restart
```

`stable` is the latest release. On amd64 and arm64 the install downloads its
binary; elsewhere it builds from source with Go 1.24+. On Windows the download
needs Git for Windows' Bash, and Go without it. The second line starts the
plugin now rather than at Herdr's next start.

To update, keeping your configuration:

```sh
herdr plugin update herdr.auto-title
herdr plugin action invoke herdr.auto-title.restart
```

Herdr before 0.9.4 has no `plugin update`; rerun the install lines instead.
An update stays on the ref you installed: a tag never moves, and no `--ref`
follows `main`. To switch, install once with `--ref stable`.

To run the latest commit on `main`, which can be ahead of the latest release,
leave `--ref` out. Herdr then builds from source with Go 1.24+:

```sh
herdr plugin install kryptamine/herdr-auto-title
herdr plugin action invoke herdr.auto-title.restart
```

## Herdr desktop app

The [desktop app](https://herdr.dev/) has no plugins, so Auto Title runs there
as a service of its own, started at every login on macOS and Linux. Windows is
not supported yet.

**With the mod:** ask the app's agent to add the mod in this repository's
[`mod/`](mod/) folder, or copy `mod/mods/auto-title` into
`~/.config/herdr-app/mods/` yourself. A strip appears under the terminals; press
`Install`. In a tab of its own it downloads the binary of the release the mod
belongs to, checks it against `SHA256SUMS` and has it install itself. The same
strip starts, updates and uninstalls it; once all is well, `×` hides it until
something needs you again.

**By hand:** with a release binary, or one built with `make build`:

```sh
./herdr-auto-title desktop install     # start now and at every login
./herdr-auto-title desktop uninstall   # stop it and remove it
```

Installing again is the update. The service runs its own copy of the binary, so
the one you ran can be deleted. Its log is `~/Library/Logs/herdr-auto-title.log`
on macOS and `journalctl --user -u herdr-auto-title` on Linux.

What differs from Herdr TUI:

- Only tabs are named: the desktop app has no pane labels or workspace topics.
- The service does not see your shell's environment, so settings go in
  [`config.env`](#configuration), never in a shell profile.
- `HERDR_AUTO_TITLE_ICONS` needs a Nerd Font as the app's `ui_font` in its
  `settings.json`, since the tab bar is drawn in that font.

## Recommended setup

Three settings make Auto Title work as intended. Each is optional.

**Create tabs without a name prompt.** Herdr asks for a name before it creates
a tab, and a name typed there counts as your own, so Auto Title never names
that tab. In Herdr's `config.toml`:

```toml
[ui]
prompt_new_tab_name = false
```

**Install Herdr's integration for each agent you use.** It tells Auto Title
which session a pane holds, so the tab is named after that session. Without it,
a tab says only what the agent puts in its terminal title, and for Antigravity
CLI that is nothing but `agy`.

```sh
herdr integration install claude           # Claude Code
herdr integration install codex            # Codex
herdr integration install antigravity-cli  # Antigravity CLI
herdr integration install grok             # Grok Build
herdr integration install hermes           # Hermes Agent
herdr integration install pi               # Pi
```

**Show the topic in the sidebar.** Auto Title reports what each workspace's
active tab is doing as `$topic`, and Herdr draws it only where a sidebar row
asks for it. This keeps the name and branch on top, with the topic in bold
under them:

```toml
[ui.sidebar.spaces]
rows = [
  ["state_icon", "workspace", "branch", "git_status"],
  [{ token = "$topic", bold = true, dim = false }],
]
```

Each client reads its own `config.toml`, so set the rows on every machine you
attach from.

## What you get

```
~/work/dashboard                       →  1 · dashboard
~/work/dashboard on feature/MC-13200   →  2 · dashboard › MC-13200
nvim editing auth.provider.ts          →  3 · nvim › auth.provider.ts
an agent working on OAuth scopes       →  4 · dashboard › Implement OAuth scopes
ssh into prod-01                       →  5 · ssh › prod-01
mosh into devbox                       →  6 · mosh › devbox
$HOME                                  →  7 · Shell
```

- The number in front is the tab's position, which is also the key that
  switches to it.
- The default branch is left out. Other branches are cut down to what
  identifies them: `bugfix-asa-cpanel-uapi-mc-13675` becomes `MC-13675`.
- A tab with several panes is named after the pane focused in it, whether the
  tab is shown or not. `HERDR_AUTO_TITLE_PREFER_AGENT=true` names it after its
  agent pane instead.
- Each pane gets a name of its own, so Herdr's goto panel (`prefix`+`g`) no
  longer lists every Claude Code pane as `claude`.
- Rename a tab or a pane yourself and Auto Title stops touching it. Clear the
  name to hand it back.
- On Windows, Herdr reports only the shell or an agent running in a pane, so an
  editor or an ssh or mosh session does not name its tab.

> [!WARNING]
> In the default mode, the first start renames every pane, including panes you
> had already named by hand. A pane you rename after that is left alone.
> Session-only mode also protects existing names at startup.

## Configuration

Every setting is optional. Copy [`config.env.example`](config.env.example) to
`~/.config/herdr-auto-title/config.env` and uncomment what you need. That path
works on every platform: on Windows it is under `%USERPROFILE%\.config`. Then
[restart](#restarting): the file is read once at startup.

Two other directories are read, and the first that already holds the file
wins: `$XDG_CONFIG_HOME`, when it is set to an absolute path, comes before
`~/.config`, and `~/Library/Application Support` on macOS or `%APPDATA%` on
Windows comes after it. The config directory that `herdr plugin list` prints is
not read.

| Setting                         | Default                                  | What it does                                                       |
| ------------------------------- | ---------------------------------------- | ------------------------------------------------------------------ |
| `HERDR_AUTO_TITLE_DEBUG`        | `false`                                  | Log at DEBUG instead of INFO                                       |
| `HERDR_AUTO_TITLE_POLL_MS`      | `500`                                    | How often the session is read, in milliseconds                     |
| `HERDR_AUTO_TITLE_MAX_LENGTH`   | `50`                                     | Longest title, in columns                                          |
| `HERDR_AUTO_TITLE_BRANCH_MAX`   | `12`                                     | Longest branch in a title, in columns; `0` hides branches          |
| `HERDR_AUTO_TITLE_DIRECTORY_MAX` | none                                   | Longest directory in a tab title, in columns; `0` hides it where something else names the tab. Pane labels keep theirs |
| `HERDR_AUTO_TITLE_POSITION`     | `true`                                   | Put the tab's position in front of its title                       |
| `HERDR_AUTO_TITLE_MANUAL_FILE`  | `manual-names.json`, found the same way  | Where names you set by hand are kept; empty keeps them in memory   |
| `HERDR_AUTO_TITLE_TRANSCRIPT`   | `true`                                   | Read an agent's own session files: what it is doing, and where     |
| `HERDR_AUTO_TITLE_SESSION_ONLY` | `false`                                | Mirror session names only; keep labels when no usable name is ready |
| `HERDR_AUTO_TITLE_AGENT_NAME`   | `false`                                  | Put the agent's name in front of what it is doing                  |
| `HERDR_AUTO_TITLE_PANES`        | `true`                                   | Name panes as well as tabs                                         |
| `HERDR_AUTO_TITLE_PREFER_AGENT` | `false`                                  | Name a tab after its agent pane even while another pane is focused |
| `HERDR_AUTO_TITLE_PANE_ID`      | `false`                                  | Put the pane's Herdr ID in front of its label, as `[w1:p2] api`    |
| `HERDR_AUTO_TITLE_AGENT_HANDLE` | `false`                                  | Put the name `herdr agent rename` gave the pane's agent in front of its label, as `driver › api` |
| `HERDR_AUTO_TITLE_ICONS`        | `false`                                  | Put a Nerd Font 3.0+ icon for what a pane runs in front of labels  |
| `HERDR_AUTO_TITLE_AGENT_STATUS` | `false`                                  | Put the tab's agent status in front of its title: `◉` idle, `◐` working, `●` blocked, `✓` done |
| `HERDR_AUTO_TITLE_WORKSPACES`   | `true`                                   | Report what each workspace's active tab is doing as its `topic`    |
| `HERDR_AUTO_TITLE_WORKSPACE_MAX_LENGTH` | none: Herdr fits it              | Longest topic, in columns                                          |
| `HERDR_AUTO_TITLE_CLAUDE_DIRS`  | none                                     | Extra Claude config homes to search, `:`-separated                 |
| `HERDR_AUTO_TITLE_HERMES_HOMES` | none                                     | Extra absolute Hermes homes to search, `:`-separated (`;` on Windows), including each home's named profiles |
| `HERDR_AUTO_TITLE_FOREIGN_PROCESSES` | none                                 | Programs that own their tab and pane labels, so Auto Title leaves them alone; herdr-mirror's streamer is built in. `:`-separated |

On Windows Herdr reports only the shell or a recognised agent per pane, so this setting and the built-in herdr-mirror check may not detect a label-owning program there.

Set `HERDR_AUTO_TITLE_SESSION_ONLY=true` to name tabs and panes from session
names only. Hermes requires `title_source=llm` or `user`; Claude uses a custom
or generated title, Pi its session name, and Codex its index name. Opening-prompt
fallbacks are skipped. Antigravity uses its annotation title and Grok Build its
summary's `generated_title`. These other agents do not expose Hermes's provenance
check, so a stored name may still contain prompt text. Missing or generic names
keep the existing label, including after exit or `/new`; terminal titles,
processes and directories do not substitute. Position, pane ID, icon and status
settings still apply to usable names. Existing user names are protected even on
the first poll, and those locks are ordinary ones: turning the mode off again
does not release them, but clearing the name does. The labels Auto Title set
are saved in both modes, after the first poll and after any poll that renamed
something, so a restart or mode switch can update them, provided the
name-state file is writable. That file therefore also holds generated names,
which can come from what an agent session is about. An empty
`HERDR_AUTO_TITLE_MANUAL_FILE` keeps ownership in memory only. The default
remains normal naming, and it never locks a name on its first poll.
`HERDR_AUTO_TITLE_TRANSCRIPT=false` leaves session-only mode with no names to
read, so Auto Title warns and renames nothing. Workspace topics retain their own resolver, but share session
reads and therefore also skip these opening-prompt fallbacks.

Hermes session titles use the native `sessions.title` in `state.db`, not chat
messages or a new LLM call. They need a Herdr integration reporting the Hermes
session ID. Linux first uses the foreground process's `HERMES_HOME` when readable;
otherwise Auto Title searches the plugin's `HERMES_HOME`, the configured extra
homes, and `~/.hermes`, including their `profiles/*` homes. A session ID found in
multiple homes is ambiguous and falls back rather than choosing another profile.
Use absolute paths, not `~`, in configuration. Details and limits are in
[Hermes session titles](docs/architecture/hermes-session-titles.md).

Turning `HERDR_AUTO_TITLE_TRANSCRIPT` off also disables Hermes database reads.
It also drops the branch from a tab whose
agent is working in a git worktree, because the transcript is what says which
worktree that is.

## Restarting

```sh
herdr plugin action invoke herdr.auto-title.restart
```

This starts a fresh Auto Title in place of the running one, and a notification
says how it went. Run it after an update, after changing the configuration, or
when the plugin has stopped naming tabs.

On Windows the action needs Herdr 0.9.0 or newer. An upgrade from a version
before 0.8.0 needs one `herdr server stop` instead: that instance does not know
to leave.

## Documentation

- [Architecture](docs/architecture/): how the plugin works and why.
- [Development](docs/development.md): working on the plugin.

## Contributors

<a href="https://github.com/recih"><img src="https://images.weserv.nl/?url=github.com/recih.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="recih"></a>
<a href="https://github.com/6temes"><img src="https://images.weserv.nl/?url=github.com/6temes.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Daniel López Prat"></a>
<a href="https://github.com/bartekbp"><img src="https://images.weserv.nl/?url=github.com/bartekbp.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Bartosz Polnik"></a>
<a href="https://github.com/xiaoyu2er"><img src="https://images.weserv.nl/?url=github.com/xiaoyu2er.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Yanqi Zong"></a>
<a href="https://github.com/youngxguo"><img src="https://images.weserv.nl/?url=github.com/youngxguo.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Young Guo"></a>
<a href="https://github.com/chechunhsu"><img src="https://images.weserv.nl/?url=github.com/chechunhsu.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Kelvin Hsu"></a>
<a href="https://github.com/2451965602"><img src="https://images.weserv.nl/?url=github.com/2451965602.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="悠然"></a>
<a href="https://github.com/beefyhalo"><img src="https://images.weserv.nl/?url=github.com/beefyhalo.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Kevin Horlick"></a>
<a href="https://github.com/cliph"><img src="https://images.weserv.nl/?url=github.com/cliph.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Cliph (Cliff Flood)"></a>
<a href="https://github.com/wapenshaw"><img src="https://images.weserv.nl/?url=github.com/wapenshaw.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Siddharth Abbineni"></a>
<a href="https://github.com/moisesmorillo"><img src="https://images.weserv.nl/?url=github.com/moisesmorillo.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Moisés Morillo"></a>
<a href="https://github.com/evenfierro-qbit"><img src="https://images.weserv.nl/?url=github.com/evenfierro-qbit.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Even Fierro"></a>
<a href="https://github.com/WillowMist"><img src="https://images.weserv.nl/?url=github.com/WillowMist.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="WillowMist"></a>
<a href="https://github.com/gandriyko"><img src="https://images.weserv.nl/?url=github.com/gandriyko.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Andrew G"></a>
<a href="https://github.com/jimbb"><img src="https://images.weserv.nl/?url=github.com/jimbb.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Jimbb"></a>
<a href="https://github.com/ModoloDev"><img src="https://images.weserv.nl/?url=github.com/ModoloDev.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Lucas Modolo"></a>
<a href="https://github.com/freethinkel"><img src="https://images.weserv.nl/?url=github.com/freethinkel.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Egor Kolesnikov"></a>
<a href="https://github.com/markdarling"><img src="https://images.weserv.nl/?url=github.com/markdarling.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Mark Darling"></a>
