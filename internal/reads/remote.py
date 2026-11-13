#!/usr/bin/env python3
"""Read remote panes' checkouts and directly owned Codex names without a shell."""

from __future__ import annotations

import json
import re
import sqlite3
import sys
from contextlib import closing
from pathlib import Path
from typing import TypedDict, cast

REF_LIMIT = 4096
INDEX_LIMIT = 8 * 1024 * 1024
INPUT_LIMIT = 8 * 1024 * 1024
SESSION_ID = re.compile(r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}")


class Request(TypedDict):
    pid: int
    dir: str
    read_sessions: bool
    read_branches: bool


class Checkout(TypedDict):
    branch: str
    commit: str
    default: str
    common_dir: str


class Reply(TypedDict):
    topic: str
    git: Checkout


def empty_checkout() -> Checkout:
    return {"branch": "", "commit": "", "default": "", "common_dir": ""}


def ref_line(path: Path) -> str:
    try:
        with path.open("rb") as stream:
            return stream.read(REF_LIMIT).decode("utf-8").split("\n", 1)[0].strip()
    except (OSError, UnicodeError):
        return ""


def git_dirs(directory: Path) -> tuple[Path, Path] | None:
    if not directory.is_absolute() or not directory.is_dir():
        return None
    for root in (directory, *directory.parents):
        marker = root / ".git"
        if marker.is_dir():
            common = marker.resolve()
            return common, common
        if not marker.is_file():
            continue
        line = ref_line(marker)
        if not line.startswith("gitdir:"):
            return None
        target = line.removeprefix("gitdir:").strip()
        if not target:
            return None
        own = Path(target)
        if not own.is_absolute():
            own = root / own
        own = own.resolve()
        shared = ref_line(own / "commondir")
        common = Path(shared) if shared else own
        if not common.is_absolute():
            common = own / common
        return own, common.resolve()
    return None


def checkout(directory: str) -> Checkout:
    found = git_dirs(Path(directory)) if directory else None
    if found is None:
        return empty_checkout()
    own, common = found
    head = ref_line(own / "HEAD")
    branch, commit = "", ""
    if head.startswith("ref: refs/heads/"):
        branch = head.removeprefix("ref: refs/heads/")
    else:
        for backend in ("rebase-merge", "rebase-apply"):
            rebasing = ref_line(own / backend / "head-name")
            if rebasing.startswith("refs/heads/"):
                branch = rebasing.removeprefix("refs/heads/")
                break
        if not branch and re.fullmatch(r"(?:[0-9a-fA-F]{40}|[0-9a-fA-F]{64})", head):
            commit = head[:7]
    if not branch and not commit:
        return empty_checkout()
    default = ref_line(common / "refs" / "remotes" / "origin" / "HEAD")
    default = (
        default.removeprefix("ref: refs/remotes/origin/") if default.startswith("ref: refs/remotes/origin/") else ""
    )
    return {"branch": branch, "commit": commit, "default": default, "common_dir": str(common)}


def held_rollouts(pid: int) -> set[Path]:
    if pid <= 0 or sys.platform != "linux":
        return set()
    root = Path("/proc") / str(pid)
    try:
        with (root / "cmdline").open("rb") as stream:
            argv = stream.read(REF_LIMIT).split(b"\0")
        # A shared daemon's files do not belong to the terminal that started it.
        if any(argument.startswith(b"app-server") for argument in argv):
            return set()
        descriptors = list((root / "fd").iterdir())
    except OSError:
        return set()
    paths = set()
    for descriptor in descriptors:
        try:
            target = descriptor.readlink()
        except OSError:
            continue
        if target.name.startswith("rollout-") and target.suffix == ".jsonl" and SESSION_ID.search(target.name):
            paths.add(target)
    return paths


def index_name(home: Path, session_id: str) -> str | None:
    try:
        with (home / "session_index.jsonl").open("rb") as stream:
            size = stream.seek(0, 2)
            stream.seek(max(0, size - INDEX_LIMIT))
            if size > INDEX_LIMIT:
                stream.readline()
            raw = stream.read(INDEX_LIMIT)
    except OSError:
        return None
    name = None
    for line in raw.splitlines():
        try:
            record = json.loads(line)
        except (ValueError, UnicodeError):
            continue
        if isinstance(record, dict) and record.get("id") == session_id:
            value = record.get("thread_name")
            if isinstance(value, str):
                name = value.strip()
    return name


def database_name(home: Path, session_id: str) -> str:
    databases = sorted(
        (path for path in home.glob("state_*.sqlite") if re.fullmatch(r"state_\d+\.sqlite", path.name)),
        key=lambda path: int(path.stem.removeprefix("state_")),
        reverse=True,
    )
    for path in databases:
        try:
            with closing(sqlite3.connect(path.resolve().as_uri() + "?mode=ro", uri=True, timeout=0.05)) as database:
                columns = {row[1] for row in database.execute("PRAGMA table_info(threads)")}
                for column in ("name", "title"):
                    if column not in columns:
                        continue
                    row = database.execute(f'SELECT "{column}" FROM threads WHERE id = ?', (session_id,)).fetchone()
                    if row and isinstance(row[0], str) and row[0].strip():
                        return str(row[0]).strip()
        except (OSError, sqlite3.Error):
            continue
    return ""


def native_name(pid: int) -> str:
    rollouts = held_rollouts(pid)
    # A cwd or a newer rollout cannot disambiguate concurrently held sessions.
    if len(rollouts) != 1:
        return ""
    path = next(iter(rollouts))
    match = SESSION_ID.search(path.name)
    if match is None:
        return ""
    home = next((parent.parent for parent in path.parents if parent.name == "sessions"), None)
    if home is None:
        return ""
    name = index_name(home, match.group())
    return name if name is not None else database_name(home, match.group())


def read(request: Request, checkouts: dict[str, Checkout] | None = None) -> Reply:
    if checkouts is None:
        checkouts = {}
    git = empty_checkout()
    if request["read_branches"]:
        directory = request["dir"]
        if directory not in checkouts:
            checkouts[directory] = checkout(directory)
        git = checkouts[directory]
    return {
        "topic": native_name(request["pid"]) if request["read_sessions"] else "",
        "git": git,
    }


def main() -> None:
    requests = cast(dict[str, Request], json.loads(sys.stdin.buffer.read(INPUT_LIMIT)))
    checkouts: dict[str, Checkout] = {}
    print(json.dumps({pane: read(request, checkouts) for pane, request in requests.items()}, ensure_ascii=False))


if __name__ == "__main__":
    main()
