#!/usr/bin/env python3
"""Local releases and PATHs shared by the installer tests."""

from __future__ import annotations

import hashlib
import http.server
import io
import json
import os
import pathlib
import shutil
import subprocess
import sys
from typing import Any, BinaryIO

WINDOWS = sys.platform == "win32"


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    """Serves the releases, and a server error for any file beside a BROKEN one."""

    def send_head(self) -> io.BytesIO | BinaryIO | None:
        if (pathlib.Path(self.translate_path(self.path)).parent / "BROKEN").exists():
            self.send_error(500)
            return None
        return super().send_head()

    def log_message(self, format: str, *args: Any) -> None:
        pass


def publish(
    releases: pathlib.Path, where: str, asset: pathlib.Path, commit: str, tamper: bool = False, broken: bool = False
) -> None:
    """Lay a release out under releases/where the way GitHub serves one."""
    directory = releases / where
    directory.mkdir(parents=True)
    if broken:
        (directory / "BROKEN").touch()
    shutil.copy(asset, directory / asset.name)
    digest = hashlib.sha256(asset.read_bytes()).hexdigest()
    if tamper:
        digest = "0" * 64
    (directory / "SHA256SUMS").write_text(f"{digest}  {asset.name}\n")
    (directory / "COMMIT").write_text(commit + "\n")


def without_go(work: pathlib.Path, also: tuple[str, ...] = ()) -> str:
    """A PATH holding the system's own tools and git, and no Go toolchain."""
    if WINDOWS:
        system = os.environ.get("SYSTEMROOT", r"C:\Windows")
        git = shutil.which("git")
        if git is None:
            raise SystemExit("git is not on PATH")
        directories = [system + r"\System32", system]
        return os.pathsep.join([*directories, str(pathlib.Path(git).parent)])
    # Some runners keep Go in /usr/bin, so the system tools are linked one by one.
    tools = work / ("tools-without-" + "-".join(("go", *also)))
    tools.mkdir(exist_ok=True)
    hidden = {"go", "gofmt", *also}
    for directory in ("/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/local/bin", "/opt/homebrew/bin"):
        if not pathlib.Path(directory).is_dir():
            continue
        for tool in pathlib.Path(directory).iterdir():
            link = tools / tool.name
            # exists() follows the link, so a dangling tool seen twice through
            # /bin -> /usr/bin would be linked a second time and fail.
            if tool.name not in hidden and not (link.is_symlink() or link.exists()):
                link.symlink_to(tool)
    return str(tools)


def git_shim(work: pathlib.Path) -> pathlib.Path:
    """Forward Git from a directory unrelated to its installation, as Scoop does."""
    directory = work / "Git shim with spaces"
    directory.mkdir()
    git = shutil.which("git")
    assert git is not None
    source = directory / "shim.go"
    source.write_text(
        'package main\nimport ("os"; "os/exec")\nfunc main() {\n'
        f"cmd := exec.Command({json.dumps(git)}, os.Args[1:]...)\n"
        "cmd.Stdin = os.Stdin; cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr\n"
        "if err := cmd.Run(); err != nil { os.Exit(1) }\n}\n"
    )
    subprocess.run(["go", "build", "-o", str(directory / "git.exe"), str(source)], check=True)
    return directory
