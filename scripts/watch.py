#!/usr/bin/env python3
"""Rebuild and restart Auto Title whenever a source file changes.

Keep it in the foreground: Ctrl+C stops both the watcher and the plugin, and
backgrounding it is how you end up with a stray process renaming tabs.
"""

from __future__ import annotations

import contextlib
import os
import pathlib
import signal
import subprocess
import sys
import time
from collections.abc import Iterator
from types import FrameType
from typing import NoReturn

ROOT = pathlib.Path(__file__).resolve().parent.parent
BINARY = ROOT / "herdr-auto-title"
PACKAGE = "./cmd/herdr-auto-title"

# How often the sources are looked at. Long enough to be free, short enough
# that a save is followed by a restart before you have switched tabs.
INTERVAL = 1.0

# How long a plugin gets to shut down before it is killed outright.
GRACE = 5.0


def sources() -> Iterator[pathlib.Path]:
    """Every file a rebuild depends on."""
    yield ROOT / "go.mod"
    for path in ROOT.rglob("*.go"):
        if ".git" not in path.parts:
            yield path


def fingerprint() -> tuple[tuple[str, int], ...]:
    """A cheap summary of when the sources were last touched."""
    stamps = []
    for path in sorted(sources()):
        # The file may go away between being listed and being read.
        with contextlib.suppress(OSError):
            stamps.append((str(path), path.stat().st_mtime_ns))
    return tuple(stamps)


def build() -> bool:
    """Compile the plugin, reporting whether the binary is now fresh."""
    build = subprocess.run(["go", "build", "-o", str(BINARY), PACKAGE], cwd=ROOT)
    return build.returncode == 0


def start() -> subprocess.Popen[bytes]:
    """Run the plugin with DEBUG logging, in this process's own terminal."""
    env = {**os.environ, "HERDR_AUTO_TITLE_DEBUG": "1"}
    return subprocess.Popen([str(BINARY)], cwd=ROOT, env=env)


def stop(plugin: subprocess.Popen[bytes] | None) -> None:
    """Stop the plugin and wait for it, so no instance outlives the watcher."""
    if plugin is None:
        return
    plugin.terminate()
    try:
        plugin.wait(timeout=GRACE)
    except subprocess.TimeoutExpired:
        plugin.kill()
        plugin.wait()


def clock() -> str:
    return time.strftime("%H:%M:%S")


def watch() -> None:
    print("--- watching for changes; Ctrl+C to stop")
    plugin: subprocess.Popen[bytes] | None = None
    last: tuple[tuple[str, int], ...] | None = None
    try:
        while True:
            current = fingerprint()
            if current != last:
                last = current
                stop(plugin)
                plugin = None
                if build():
                    plugin = start()
                    print(f"--- {clock()} started")
                else:
                    print(f"--- {clock()} build failed; nothing is running")
            elif plugin is not None and plugin.poll() is not None:
                # A plugin that stopped on its own would otherwise be missed
                # until the next edit, and the tabs would quietly stop moving.
                code = plugin.returncode
                print(f"--- {clock()} plugin exited ({code}); waiting for a change")
                plugin = None
            time.sleep(INTERVAL)
    finally:
        print()
        print("--- stopping")
        stop(plugin)


def interrupt(signum: int, frame: FrameType | None) -> NoReturn:
    """Turn a signal into the interrupt Ctrl+C already raises, so `make stop`
    runs the cleanup instead of ending the process and orphaning the plugin.
    """
    raise KeyboardInterrupt


def main() -> int:
    signal.signal(signal.SIGTERM, interrupt)
    with contextlib.suppress(KeyboardInterrupt):
        watch()
    return 0


if __name__ == "__main__":
    sys.exit(main())
