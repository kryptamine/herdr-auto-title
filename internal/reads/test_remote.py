"""Exercise remote session ownership, read options and checkout freshness."""

from __future__ import annotations

import json
import sqlite3
import sys
import tempfile
import unittest
from contextlib import closing
from pathlib import Path
from unittest.mock import patch

from internal.reads import remote

SESSION = "8852bfe0-8b24-4a23-a35e-7521d04da061"
OTHER = "0c3a1d94-77b1-4f2e-8a6d-5e91b2c4d803"


class RemoteTests(unittest.TestCase):
    def setUp(self) -> None:
        temporary = tempfile.TemporaryDirectory()
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)

    def file(self, relative: str, content: str) -> Path:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def rollout(self, session: str = SESSION) -> Path:
        return self.file(f"codex home/sessions/2026/11/12/rollout-2026-11-12T10-00-00-{session}.jsonl", "")

    def database(self, title: str, version: int = 5, session: str = SESSION) -> Path:
        path = self.root / "codex home" / f"state_{version}.sqlite"
        path.parent.mkdir(parents=True, exist_ok=True)
        with closing(sqlite3.connect(path)) as connection:
            connection.execute("CREATE TABLE threads (id TEXT PRIMARY KEY, title TEXT)")
            connection.execute("INSERT INTO threads VALUES (?, ?)", (session, title))
            connection.commit()
        return path

    def test_native_name_uses_the_exact_held_session(self) -> None:
        path = self.rollout()
        self.database("Exact native name")
        self.database("Other conversation", version=11, session=OTHER)
        with patch.object(remote, "held_rollouts", return_value={path}):
            self.assertEqual(remote.native_name(123), "Exact native name")

    def test_two_held_rollouts_are_ambiguous_even_in_one_directory(self) -> None:
        first, second = self.rollout(), self.rollout(OTHER)
        self.database("Do not choose this by time")
        with patch.object(remote, "held_rollouts", return_value={first, second}):
            self.assertEqual(remote.native_name(123), "")

    def test_index_rename_and_clear_are_read_again(self) -> None:
        path = self.rollout()
        self.database("Database fallback")
        index = path.parents[4] / "session_index.jsonl"
        with patch.object(remote, "held_rollouts", return_value={path}):
            for name in ("First name", "Changed name", ""):
                index.write_text(json.dumps({"id": SESSION, "thread_name": name}) + "\n", encoding="utf-8")
                self.assertEqual(remote.native_name(123), name)

    @unittest.skipUnless(sys.platform == "linux", "the direct process probe uses Linux /proc")
    def test_a_shared_daemon_never_supplies_a_native_name(self) -> None:
        path = self.rollout()
        root = self.root / "proc" / "123"
        (root / "fd").mkdir(parents=True)
        (root / "fd" / "4").symlink_to(path)
        with patch.object(
            remote, "Path", side_effect=lambda value: self.root / "proc" if value == "/proc" else Path(value)
        ):
            for argv in (b"codex\0app-server-daemon\0", b"codex\0--config\0example=true\0app-server\0"):
                (root / "cmdline").write_bytes(argv)
                self.assertEqual(remote.held_rollouts(123), set())
            (root / "cmdline").write_bytes(b"codex\0")
            self.assertEqual(remote.held_rollouts(123), {path})

    def test_linked_worktree_and_rebase_keep_their_branch(self) -> None:
        own = self.file("repo/.git/worktrees/topic/HEAD", "a" * 40)
        self.file("repo/.git/worktrees/topic/commondir", "../..")
        self.file("repo/.git/worktrees/topic/rebase-merge/head-name", "refs/heads/feature/topic")
        self.file("repo/.git/HEAD", "ref: refs/heads/main")
        self.file("repo/.git/refs/remotes/origin/HEAD", "ref: refs/remotes/origin/main")
        marker = self.file("worktree with spaces and an apostrophe's/.git", f"gitdir: {own.parent}")
        checkout = remote.checkout(str(marker.parent))
        self.assertEqual(checkout["branch"], "feature/topic")
        self.assertEqual(checkout["default"], "main")
        self.assertEqual(checkout["common_dir"], str((self.root / "repo" / ".git").resolve()))
        self.assertEqual(checkout["common_dir"], remote.checkout(str(self.root / "repo"))["common_dir"])

    def test_detached_and_invalid_heads(self) -> None:
        head = self.file("repo/.git/HEAD", "ab" * 20)
        self.assertEqual(remote.checkout(str(head.parents[1]))["commit"], "abababa")
        head.write_text("not a commit", encoding="utf-8")
        self.assertEqual(remote.checkout(str(head.parents[1])), remote.empty_checkout())
        self.assertEqual(remote.checkout(str(head.parents[1] / "missing")), remote.empty_checkout())

    def test_checkout_memo_lasts_only_one_batch(self) -> None:
        head = self.file("repo/.git/HEAD", "ref: refs/heads/first")
        request: remote.Request = {
            "pid": 0,
            "dir": str(head.parents[1]),
            "read_sessions": False,
            "read_branches": True,
        }
        memo: dict[str, remote.Checkout] = {}
        self.assertEqual(remote.read(request, memo)["git"]["branch"], "first")
        head.write_text("ref: refs/heads/second", encoding="utf-8")
        self.assertEqual(remote.read(request, memo)["git"]["branch"], "first")
        self.assertEqual(remote.read(request)["git"]["branch"], "second")

    def test_disabled_reads_touch_neither_sessions_nor_checkouts(self) -> None:
        request: remote.Request = {"pid": 123, "dir": "/remote", "read_sessions": False, "read_branches": False}
        with (
            patch.object(remote, "native_name", side_effect=AssertionError("session read disabled")),
            patch.object(remote, "checkout", side_effect=AssertionError("checkout read disabled")),
        ):
            self.assertEqual(remote.read(request), {"topic": "", "git": remote.empty_checkout()})


if __name__ == "__main__":
    unittest.main()
