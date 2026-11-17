#!/usr/bin/env python3
"""Check that the machine helper answers each question with facts alone."""

from __future__ import annotations

import base64
import io
import json
import sys
import tempfile
import unittest
from pathlib import Path
from typing import Any
from unittest import mock

from internal.files import machine


def ask(questions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    stdin = io.TextIOWrapper(io.BytesIO(json.dumps(questions).encode()))
    stdout = io.StringIO()
    with mock.patch.object(sys, "stdin", stdin), mock.patch.object(sys, "stdout", stdout):
        machine.main()
    answers: list[dict[str, Any]] = json.loads(stdout.getvalue())
    return answers


class HelperTest(unittest.TestCase):
    def setUp(self) -> None:
        self.dir = Path(tempfile.mkdtemp())
        (self.dir / "HEAD").write_bytes(b"ref: refs/heads/main\n\xff")

    def test_answers_come_back_in_the_order_asked(self) -> None:
        answers = ask(
            [
                {"op": "stat", "path": str(self.dir)},
                {"op": "stat", "path": str(self.dir / "gone")},
                {"op": "read", "path": str(self.dir / "HEAD"), "from": 5, "limit": 4},
            ]
        )
        self.assertTrue(answers[0]["dir"])
        self.assertTrue(answers[1]["missing"])
        self.assertEqual(base64.b64decode(answers[2]["data"]), b"refs")

    def test_bytes_are_returned_undecoded(self) -> None:
        # What a ref file holds is decided in Go, the same for both routes.
        (answer,) = ask([{"op": "read", "path": str(self.dir / "HEAD"), "from": 0, "limit": 64}])
        self.assertEqual(base64.b64decode(answer["data"]), b"ref: refs/heads/main\n\xff")

    def test_links_report_their_targets(self) -> None:
        (self.dir / "3").symlink_to(self.dir / "HEAD")
        (answer,) = ask([{"op": "links", "path": str(self.dir)}])
        # Windows spells a link's target with a \\?\ prefix; Linux does not.
        (target,) = answer["names"]
        self.assertTrue(Path(target).samefile(self.dir / "HEAD"))

    def test_a_malformed_question_fails_alone(self) -> None:
        answers = ask([{"op": "stat"}, {"op": "unknown", "path": "/"}, {"op": "glob", "path": str(self.dir / "*")}])
        self.assertTrue(answers[0]["failed"])
        self.assertTrue(answers[1]["failed"])
        self.assertEqual(answers[2]["names"], [str(self.dir / "HEAD")])


if __name__ == "__main__":
    unittest.main()
