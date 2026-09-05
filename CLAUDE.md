# CLAUDE.md

@AGENTS.md

The canonical onboarding is **AGENTS.md**, imported above. Only what is specific
to Claude Code is below.

## Tooling

- `validate-agent-work` — the skill to run after any change, before handing work
  back. It runs `make check` and reads the diff against the mandatory rules.
- Delegate a broad multi-file search to the **Explore** agent and keep the
  conclusion, not the file dumps.

## MacOS edit note

BSD `sed -i` differs from GNU sed — prefer `perl -pi -e 's/old/new/g' file` for
in-place substitution.
