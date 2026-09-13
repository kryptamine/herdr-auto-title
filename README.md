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

A [Herdr](https://herdr.dev) plugin that names your tabs and panes after the
work in them. It reads the session twice a second, and it leaves alone any tab
or pane you rename yourself.

https://github.com/user-attachments/assets/606fde6a-dfd3-4010-b4f3-80c79d74ea63

## Install

You need Herdr 0.8.2+ and Go 1.24+ on macOS, Linux or Windows. Herdr builds the
plugin from source when it installs it.

```sh
herdr plugin install kryptamine/herdr-auto-title
herdr server stop   # closes the session; `herdr` brings it back
```

> [!IMPORTANT]
> Herdr starts plugins only when its server starts, so nothing is renamed until
> you stop the server. Reopening the terminal attaches a new client to the same
> server and does not help.

If you use Claude Code, also run `herdr integration install claude`. Without it,
a session you opened with a slash command and never prompted stays `claude`.

## What you get

```
~/work/dashboard                       →  1 · dashboard
~/work/dashboard on feature/MC-13200   →  2 · dashboard › MC-13200
nvim editing auth.provider.ts          →  3 · nvim › auth.provider.ts
an agent working on OAuth scopes       →  4 · dashboard › claude › Implement OAuth scopes
ssh into prod-01                       →  5 · ssh › prod-01
$HOME                                  →  6 · Shell
```

- The number in front is the tab's position, which is also the key that
  switches to it.
- The default branch is left out. Other branches are cut down to what
  identifies them: `bugfix-asa-cpanel-uapi-mc-13675` becomes `MC-13675`.
- A tab with several panes is named after the focused pane, a pane with a busy
  agent, or the pane that changed last.
- Each pane gets a name of its own, so Herdr's goto panel (`prefix`+`g`) no
  longer lists every Claude Code pane as `claude`.
- Rename a tab or a pane yourself and Auto Title stops touching it. Clear the
  name to hand it back.
- On Windows, Herdr reports only the shell or an agent running in a pane, so an
  editor or an ssh session does not name its tab.

> [!WARNING]
> The first start renames every pane, including panes you had already named by
> hand. A pane you rename after that is left alone.

## Configuration

Every setting is optional. Copy [`config.env.example`](config.env.example) to
the path for your platform and uncomment what you need:

| Platform | File                                                        |
| -------- | ----------------------------------------------------------- |
| macOS    | `~/Library/Application Support/herdr-auto-title/config.env` |
| Linux    | `~/.config/herdr-auto-title/config.env`                     |
| Windows  | `%APPDATA%\herdr-auto-title\config.env`                     |

Auto Title reads the file once at startup, so run `herdr server stop` after a
change. It does not read the config directory that `herdr plugin list` prints.

| Setting                         | Default                                  | What it does                                                       |
| ------------------------------- | ---------------------------------------- | ------------------------------------------------------------------ |
| `HERDR_AUTO_TITLE_DEBUG`        | `false`                                  | Log at DEBUG instead of INFO                                       |
| `HERDR_AUTO_TITLE_POLL_MS`      | `500`                                    | How often the session is read, in milliseconds                     |
| `HERDR_AUTO_TITLE_MAX_LENGTH`   | `50`                                     | Longest title, in columns                                          |
| `HERDR_AUTO_TITLE_BRANCH_MAX`   | `12`                                     | Longest branch in a title, in columns; `0` hides branches          |
| `HERDR_AUTO_TITLE_POSITION`     | `true`                                   | Put the tab's position in front of its title                       |
| `HERDR_AUTO_TITLE_MANUAL_FILE`  | `manual-names.json` next to `config.env` | Where names you set by hand are kept; empty keeps them in memory   |
| `HERDR_AUTO_TITLE_TRANSCRIPT`   | `true`                                   | Read Claude Code's transcript when it has not titled its terminal  |
| `HERDR_AUTO_TITLE_AGENT_NAME`   | `true`                                   | Put the agent's name in front of what it is doing                  |
| `HERDR_AUTO_TITLE_PANES`        | `true`                                   | Name panes as well as tabs                                         |
| `HERDR_AUTO_TITLE_PREFER_AGENT` | `false`                                  | Name a tab after its agent pane even while another pane is focused |

## Documentation

- [Architecture](docs/architecture/): how the plugin works and why.
- [Development](docs/development.md): working on the plugin.

## Contributors

<a href="https://github.com/recih"><img src="https://images.weserv.nl/?url=github.com/recih.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="recih"></a>
<a href="https://github.com/bartekbp"><img src="https://images.weserv.nl/?url=github.com/bartekbp.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Bartosz Polnik"></a>
<a href="https://github.com/xiaoyu2er"><img src="https://images.weserv.nl/?url=github.com/xiaoyu2er.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Yanqi Zong"></a>
<a href="https://github.com/youngxguo"><img src="https://images.weserv.nl/?url=github.com/youngxguo.png&w=128&h=128&fit=cover&mask=circle&maxage=7d" width="64" alt="Young Guo"></a>
