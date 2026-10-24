#!/usr/bin/env python3
"""Build the static release binaries into dist/ and write SHA256SUMS beside them.

The names are what the install scripts download, so renaming one sends every
install of that release to a Go build.
"""

import hashlib
import os
import pathlib
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
NAME = "herdr-auto-title"
PACKAGE = "./cmd/herdr-auto-title"

# GOOS to the system an asset is named for: macos, not darwin.
SYSTEMS = {"linux": "linux", "darwin": "macos", "windows": "windows"}
ARCHITECTURES = ("amd64", "arm64")


def asset(goos: str, goarch: str) -> str:
    suffix = ".exe" if goos == "windows" else ""
    return f"{NAME}-{SYSTEMS[goos]}-{goarch}{suffix}"


def build(dist: pathlib.Path, goos: str, goarch: str) -> str:
    name = asset(goos, goarch)
    env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
    command = ["go", "build", "-trimpath", "-ldflags=-s -w", "-o", str(dist / name), PACKAGE]
    subprocess.run(command, cwd=ROOT, env=env, check=True)
    print(f"built {name}", file=sys.stderr)
    return name


def main() -> None:
    dist = ROOT / "dist"
    dist.mkdir(exist_ok=True)
    for stale in dist.iterdir():
        stale.unlink()

    names = [build(dist, goos, goarch) for goos in SYSTEMS for goarch in ARCHITECTURES]

    lines = [f"{hashlib.sha256((dist / name).read_bytes()).hexdigest()}  {name}\n" for name in names]
    (dist / "SHA256SUMS").write_text("".join(lines))


if __name__ == "__main__":
    main()
