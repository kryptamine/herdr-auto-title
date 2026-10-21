# Hermes session titles

`internal/reads` supplies `PaneState.AgentTopic` from Hermes's native title. The
existing transcript source sanitizes and formats it, so tab/pane manual ownership,
workspace topics, truncation, terminal-title precedence and foreign-process
protection are unchanged. This is a session-title reader, not another title
generator: it reads neither prompts nor messages and makes no LLM call.

## Identity and profiles

Both the pane and its session reference must identify `hermes`, and the reference
must have `kind: "id"`. Hermes IDs use a timestamp and hexadecimal suffix, such as
`20261021_133849_f560a2`; they do not pass the UUID validator used by Claude.
`internal/hermes` validates its own IDs before lookup and binds the exact value to
`WHERE id = ?`. It never guesses a session from cwd, recency or a terminal title.
The pane's process directory still supplies its checkout; the database cwd is not
needed for naming.

On Linux the existing foreground process PID supplies `/proc/<pid>/environ`.
Only `HERMES_HOME` is retained, from at most 1 MiB of environment. It is authoritative:
a missing session/title/database in that home does not fall through to a different
profile. Only the foreground process is inspected, not tools it started.
An inaccessible environment, absent variable, or unsupported platform falls back
to configured homes. Procfs reports the process's startup environment; later
in-process environment changes are not discoverable this way.

Fallback candidates are the plugin's `HERMES_HOME`, the path-list in
`HERDR_AUTO_TITLE_HERMES_HOMES`, and `~/.hermes`, plus each root's immediate
`profiles/*` directories. Paths must be absolute; no tilde or shell expansion is
performed. Configured homes may themselves be profiles. Relative entries are
ignored. Exact cleaned duplicate paths are searched once. A session present in
two homes, even if one title is empty, is ambiguous: return nothing rather than
name a pane from another profile. An unreadable/corrupt existing candidate also
makes fallback inconclusive. Symlink aliases are not resolved and can therefore
produce a conservative ambiguous result. Profile directory symlinks are not
followed during automatic discovery; configure their absolute home explicitly.

## Bounded, read-only database access

The pure-Go `modernc.org/sqlite` driver is pinned to a Go-1.24-compatible version.
It adds dependencies and binary/build cost, but avoids requiring Python, a
SQLite executable, CGO or a subprocess per pane on every poll. Dependency changes
must keep the manifest's Go floor and the cross-platform build working.

Every lookup opens only an existing regular `state.db` using `mode=ro`, reads one
bound row and closes its handle. Missing files are not created. SQLite's normal
WAL support remains enabled: do not use `immutable=1`, which would miss live WAL
updates. SQLite may need existing WAL/shared-memory sidecars or permission to
manage shared memory; if access fails, ordinary title fallback remains available.
No write SQL is issued, and no conversation rows are queried. The stored
`title_source` is irrelevant: user and generated native titles use the same field.

A lookup has a 100 ms context budget, no SQLite busy wait, one connection at a
time, at most 64 candidate homes and at most 64 profile directory entries per
root. An over-limit search returns nothing, not a partial potentially misleading
result. SQL limits the title to 4096 characters before the existing sanitization
and display-width truncation. Context cancellation bounds SQL, not a kernel
filesystem call stalled on an unhealthy/network mount; use local homes.

No titles or connections are cached across polls. That keeps WAL changes, manual
session renames, `/new`, resumed sessions, deleted titles and replaced databases
fresh without a cache invalidation or handle-retention policy. A poll's existing
`Fill` memoization still avoids rereading a pane for its tab, pane and topic.
The database format is an internal Hermes detail; a schema change safely yields
no topic until this reader is updated. `HERDR_AUTO_TITLE_TRANSCRIPT=false` disables
these reads along with the other agent session sources.
