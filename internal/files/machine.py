#!/usr/bin/env python3
"""Answer a batch of file questions on a connected machine, knowing no format.

Each question is a stat, a bounded read, a glob or the targets of a directory's
symbolic links. What the files mean is decided in Go, the same as for local
files, so nothing parsed here can drift from the local readers.
"""

from __future__ import annotations

import base64
import glob
import json
import stat
import sys
from pathlib import Path
from typing import Any

INPUT_LIMIT = 8 * 1024 * 1024


def answer(question: dict[str, Any]) -> dict[str, Any]:
    op, name = question.get("op"), question.get("path")
    if not isinstance(name, str):
        return {"failed": True}
    path = Path(name)
    try:
        if op == "stat":
            info = path.stat()
            return {
                "size": info.st_size,
                "mtime_ns": info.st_mtime_ns,
                "dir": stat.S_ISDIR(info.st_mode),
                "regular": stat.S_ISREG(info.st_mode),
            }
        if op == "read":
            with path.open("rb") as stream:
                stream.seek(int(question.get("from", 0)))
                data = stream.read(int(question.get("limit", 0)))
            return {"data": base64.b64encode(data).decode("ascii")}
        if op == "glob":
            # Path.glob takes no absolute pattern.
            return {"names": glob.glob(name)}  # noqa: PTH207
        if op == "links":
            names: list[str] = []
            for entry in path.iterdir():
                try:
                    names.append(str(entry.readlink()))
                except OSError:
                    continue
            return {"names": names}
    except FileNotFoundError:
        return {"missing": True}
    except (OSError, ValueError):
        return {"failed": True}
    return {"failed": True}


def main() -> None:
    questions = json.loads(sys.stdin.buffer.read(INPUT_LIMIT))
    json.dump([answer(question) for question in questions], sys.stdout)


if __name__ == "__main__":
    main()
