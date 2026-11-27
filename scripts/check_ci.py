#!/usr/bin/env python3
"""Check that the CI gate job needs every other job and always runs.

The branch ruleset is meant to require the gate alone, so a job left out of its
needs never blocks a merge, and a gate that can be skipped passes when anything
fails.
"""

from __future__ import annotations

import re
import sys

from dist import ROOT

WORKFLOW = ROOT / ".github" / "workflows" / "ci.yml"
GATE = "gate"

# There is no YAML parser in the standard library, so the workflow is read by
# indentation, and any form this does not know is an error rather than a guess.
JOB = re.compile(r"^  ([A-Za-z0-9_-]+):\s*(#.*)?$")
FIELD = re.compile(r"^    (needs|if):(.*)$")
FLOW_LIST = re.compile(r"^\[([^\]]*)\]$")


def jobs(text: str) -> dict[str, list[str]] | None:
    found: dict[str, list[str]] | None = None
    current: list[str] = []
    for line in text.splitlines():
        if found is None:
            if line.rstrip() == "jobs:":
                found = {}
            continue
        if line and not line[0].isspace() and not line.startswith("#"):
            break
        job = JOB.match(line)
        if job:
            current = found.setdefault(job.group(1), [])
        else:
            current.append(line)
    return found


def problems(text: str) -> list[str]:
    found = jobs(text)
    if found is None:
        return ["no jobs: key"]
    if not found:
        return ["no jobs under jobs:"]
    if GATE not in found:
        return ["no gate job"]
    fields: dict[str, str] = {}
    for line in found[GATE]:
        field = FIELD.match(line)
        if field:
            fields[field.group(1)] = field.group(2).strip()
    result = []
    if fields.get("if") != "always()":
        result.append("the gate must run on if: always(), or a failed job skips it and it passes")
    if "needs" not in fields:
        return [*result, "the gate needs no jobs"]
    listed = FLOW_LIST.match(fields["needs"])
    if listed is None:
        return [*result, "the gate's needs must be a one-line list: needs: [a, b]"]
    needs = {name.strip() for name in listed.group(1).split(",") if name.strip()}
    others = set(found) - {GATE}
    result += [f"the gate does not need {name}" for name in sorted(others - needs)]
    result += [f"the gate needs {name}, which is not a job" for name in sorted(needs - others)]
    return result


def main() -> None:
    found = problems(WORKFLOW.read_text())
    for problem in found:
        print(f"{WORKFLOW.relative_to(ROOT)}: {problem}", file=sys.stderr)
    if found:
        sys.exit(1)


if __name__ == "__main__":
    main()
