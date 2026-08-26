---
type: doc
title: 'Architecture'
description: 'Index of the architecture notes for Herdr Auto Title: the poll loop, how a tab becomes a name, how untrusted terminal values are handled, how manual renames are protected, and the verified facts about the Herdr socket API underneath all of it.'
tags: [architecture, index]
created: 2026-08-25
generated: { by: claude-code/opus-5, at: 2026-08-25T12:46:22+03:00 }
---

# Architecture

Auto Title is a long-lived Go process that polls the Herdr session over a Unix
socket and renames tabs. These notes record how it works and, more importantly,
**why it works that way** — most of the decisions here overturned something the
originating specification assumed, and each one is recorded with the measurement
that settled it.

| Note | What it covers |
|------|----------------|
| [The poll loop](./poll-loop.md) | Why polling beats the event stream, what one poll does, what survives between polls, and how the loop behaves when Herdr is unreachable |
| [Title resolution](./title-resolution.md) | Which pane speaks for a tab, the confidence ladder, what each source contributes, why the workspace name is not repeated, and why the tab position leads the title |
| [Sanitizing untrusted values](./sanitization.md) | What is stripped from terminal-derived values, what is rejected as saying nothing, and why the length limit is counted in columns |
| [Manual rename protection](./manual-rename-protection.md) | Telling your rename from the plugin's own using nothing but successive polls, and what that costs |
| [Configuration](./configuration.md) | Why a configuration file exists at all, where it lives, why the environment overrides it, and what the file format does and does not do |
| [Herdr socket API](./herdr-socket-api.md) | The wire protocol and the measured facts about Herdr 0.8.2 that everything else rests on |

Two things are worth knowing before reading any of them:

- **Decide from freshly read state.** Each poll reads the whole session and
  throws it away. Almost nothing is carried forward, and what is carried is
  carried because a snapshot cannot express it.
- **A struct field exists only if code reads it.** Herdr's wire objects carry far
  more than Auto Title needs, and mirroring them in full would make a type claim
  a dependency the code does not have.

For working on the code — the loops, running against your own session, what each
log line means — see [development](../development.md).

These notes replace the originating specification, which was written before any
of it was measured and was wrong about enough that keeping it would have meant
keeping two answers to every question. It is in the git history if a decision
ever needs its provenance.
