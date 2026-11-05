#!/usr/bin/env python3
"""Check the Windows desktop probe without connecting to a real session."""

import contextlib
import io
import unittest
from typing import Any
from unittest.mock import patch

import probe


class DesktopProbeTest(unittest.TestCase):
    def test_windows_reads_complete_local_terminals_through_cli(self) -> None:
        responses: dict[tuple[str, ...], dict[str, Any]] = {
            ("herdr-app", "ls"): {"spaces": [{"id": "s1", "name": "test", "machine": None, "tabs": []}]},
            ("herdr-app", "status"): {"focused_pane": "p1"},
            ("hc", "server", "status"): {},
            ("hc", "ls"): {"terminals": [{"address": "t1"}, {"address": "remote/t2"}]},
            ("hc", "get", "t1"): {
                "address": "t1",
                "title": "topic",
                "foreground": {"name": "pwsh.exe"},
            },
        }
        with (
            patch.object(probe, "WINDOWS", True),
            patch.object(probe, "cli", side_effect=lambda *args: responses[args]) as cli,
            patch.object(probe, "ask", side_effect=AssertionError("Unix socket used")),
            contextlib.redirect_stdout(io.StringIO()) as output,
        ):
            probe.cmd_desktop()
        self.assertIn("focused pane p1", output.getvalue())
        self.assertEqual(cli.call_count, len(responses))


if __name__ == "__main__":
    unittest.main()
