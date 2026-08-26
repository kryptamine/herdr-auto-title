<div align="center">
  <p>
    <img src="assets/banner.png" alt="Herdr Auto Title: smarter tab titles, zero effort" width="800">
  </p>
  <p>
    <a href="https://github.com/kryptamine/herdr-auto-title/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/kryptamine/herdr-auto-title/ci.yml?branch=main&style=for-the-badge&logo=githubactions&logoColor=white&label=CI&labelColor=000000" alt="CI status"></a>
    <a href="https://github.com/kryptamine/herdr-auto-title/releases"><img src="https://img.shields.io/github/v/release/kryptamine/herdr-auto-title?style=for-the-badge&logo=github&logoColor=white&color=0797ff&labelColor=000000" alt="Latest release"></a>
    <a href="https://go.dev"><img src="https://img.shields.io/github/go-mod/go-version/kryptamine/herdr-auto-title?style=for-the-badge&logo=go&logoColor=white&color=0797ff&labelColor=000000" alt="Go version"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-0797ff?style=for-the-badge&labelColor=000000" alt="MIT licence"></a>
  </p>
</div>

A [Herdr](https://herdr.dev) plugin that reads your session twice a second and
keeps every tab's title in step with the work in it. Rename a tab yourself and
it leaves that tab alone from then on.

## Demo

https://github.com/user-attachments/assets/606fde6a-dfd3-4010-b4f3-80c79d74ea63

## Quick start

> [!NOTE]
> Requires Herdr 0.8.2+ and Go 1.24+ on macOS or Linux. Herdr compiles the
> plugin from source on your machine when it installs it.

```sh
herdr plugin install kryptamine/herdr-auto-title
```

> [!IMPORTANT]
> Installing does not start the plugin. Plugins are started by the Herdr
> **server**, and only when it restores a session, so run this once:
>
> ```sh
> herdr server stop   # closes the session; `herdr` brings it back
> ```
>
> Reopening your terminal will not do: that attaches a new client and leaves
> the server, and your plugin, exactly as they were.

Herdr clones the repository, builds the binary and registers it. Until the
server has restarted, nothing is renamed. `herdr plugin list` shows the plugin,
`herdr plugin disable` turns it off.

### Better names for agent tabs

An agent usually says what it is working on through its terminal title, and that
is where Auto Title reads it. Claude Code has one blind spot: a session you
opened with a slash command and never typed a prompt into is never given a
title, so its tab stays `claude`. One command closes it:

```sh
herdr integration install claude
```

That is Herdr's own hook, not this plugin's: it tells Herdr which session each
pane holds, and Auto Title reads the topic from the session itself.

Herdr ships the same hook for seventeen agents (`herdr integration status`) and
Auto Title accepts a session from any of them, but every agent keeps its
transcript in its own format, so **only Claude Code is read so far**. Any other
agent's tab is named from its terminal title, exactly as before, and
`HERDR_AUTO_TITLE_TRANSCRIPT=false` stops Auto Title reading transcripts at all.

## What your tabs will be called

```
~/work/dashboard                       →  1 · dashboard
~/work/dashboard on feature/MC-13200   →  2 · dashboard › MC-13200
nvim editing auth.provider.ts          →  3 · nvim › auth.provider.ts
an agent working on OAuth scopes       →  4 · dashboard › claude › Implement OAuth scopes
ssh into prod-01                       →  5 · ssh › prod-01
$HOME                                  →  6 · Shell
```

Titles read `<position> · <context> › <activity>`, capped at 50 columns of the
tab bar. The activity is the first of these that has something to say: what an
agent reports it is working on, then the terminal title, then a lone program in
the pane. The context is the directory you are in, or the machine you reached
over `ssh`, and the branch you have checked out qualifies it.

These rules explain most surprises:

- **The number in front is the tab's position in the workspace**, which is the
  key that switches to it. It leads the title because the tab bar cuts the tail
  of one too wide for it. `HERDR_AUTO_TITLE_POSITION=false` leaves it out.
- **A branch shows when it distinguishes.** Your repository's default branch
  says nothing, so it is left out; anything else is shown, shortened to what
  identifies it (`bugfix-asa-cpanel-uapi-mc-13675` → `MC-13675`).
- **A tab you renamed is yours** and Auto Title never touches it again. Renaming
  it back does not hand it over: stop the plugin, delete the tab's entry from
  `manual-names.json`, and start the plugin again.
- Paths, shell prompts, bare program names and your workspace name are left out,
  because they only repeat what the screen already shows.
- A tab with several panes takes its name from one of them: the focused pane, a
  pane running a busy agent, or the pane that changed last.

## Configuration

Everything is optional; the defaults are what the section above describes.
Settings live in a file Auto Title reads once, at startup:

| Platform | File                                                        |
| -------- | ----------------------------------------------------------- |
| macOS    | `~/Library/Application Support/herdr-auto-title/config.env` |
| Linux    | `~/.config/herdr-auto-title/config.env`                     |

Nothing creates it for you; [`config.env.example`](config.env.example) lists
every setting commented out. **A change reaches the plugin only when it
restarts**, with the same `herdr server stop` the install needs.

> [!NOTE]
> The config directory `herdr plugin list` prints is Herdr's own. Auto Title
> does not read it — a plugin you start by hand would never see it. Use the
> path above.

| Setting                        | Default                                   | What it does                                                               |
| ------------------------------ | ----------------------------------------- | -------------------------------------------------------------------------- |
| `HERDR_AUTO_TITLE_DEBUG`       | `false`                                   | Log at DEBUG rather than INFO                                              |
| `HERDR_AUTO_TITLE_POLL_MS`     | `500`                                     | How often the session is read, in milliseconds                             |
| `HERDR_AUTO_TITLE_MAX_LENGTH`  | `50`                                      | Longest title, in columns of the tab bar                                   |
| `HERDR_AUTO_TITLE_BRANCH_MAX`  | `12`                                      | Longest branch a title may carry, in columns; `0` leaves branches out      |
| `HERDR_AUTO_TITLE_POSITION`    | `true`                                    | Put each tab's position in front of its title                              |
| `HERDR_AUTO_TITLE_MANUAL_FILE` | `manual-names.json`, next to `config.env` | Where tabs you renamed by hand are remembered; empty keeps them in memory  |
| `HERDR_AUTO_TITLE_TRANSCRIPT`  | `true`                                    | Read an agent's own session transcript when it has not titled its terminal |

## Documentation

- [Architecture](docs/architecture/) — how it works and why: the poll loop, how
  a tab becomes a name, where configuration comes from, and the measured facts
  about the Herdr socket API.
- [Development](docs/development.md) — working on it.
