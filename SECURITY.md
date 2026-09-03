# Security Policy

## Supported versions

Auto Title is pre-1.0 — a fix lands on `main` and goes out in the next release
rather than as a patch to an older tag. Only the newest release is supported.

| Version | Supported |
|---------|-----------|
| 0.3.x   | ✅        |
| < 0.3   | ❌        |

## Reporting a vulnerability

Email **satretdinovalexander@gmail.com** with `herdr-auto-title` in the
subject. Do not open a public issue for anything exploitable: an issue is the
right place once a fix is out, not before.

Include what makes it reproducible:

- the plugin version (the `version` field in `herdr-plugin.toml`), your Herdr
  version and your OS;
- what an attacker controls (a directory name, a terminal title, a file an
  agent wrote) and what that gets them;
- the shortest sequence that shows it.

You get an acknowledgement within 72 hours and an assessment within a week.
This is a one-person project, so a fix takes as long as it takes, but you will
hear where it stands. Tell me whether you plan to disclose it and when, and the
release is timed to that. You get credit in the release notes unless you would
rather not be named.

## Scope

Auto Title polls a local Herdr session over a Unix socket and renames tabs. It
starts no subprocess, and that socket is the only thing it talks to. Every
value that reaches a tab title comes from terminal output and is treated as
hostile; [docs/architecture/sanitization.md](docs/architecture/sanitization.md)
says what is stripped and what is rejected.

Worth reporting:

- a terminal-derived value reaching a shell, a subprocess or a path;
- an escape, control or format character that survives `Sanitize` into a tab
  label, or a label that forges structure the session does not have;
- a read escaping Claude Code's `projects` directory: a session id arrives over
  the socket and becomes part of a transcript path;
- a crash or an unbounded read a pane can trigger on its own.

Not this project's to fix:

- vulnerabilities in Herdr itself, which go to [herdr.dev](https://herdr.dev);
- anything that presupposes an attacker who already runs code as you. Whoever
  reaches the Herdr socket owns the session with or without this plugin;
- a title that is wrong or unhelpful. That is a bug, so open an issue.
