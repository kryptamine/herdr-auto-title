#!/usr/bin/env python3
"""Run this platform's install script against releases served locally.

Each case installs a copy of the plugin and checks what ended up in its root.
The binary served is the one scripts/dist.py builds, so the two agree on names.
"""

import functools
import hashlib
import http.server
import os
import pathlib
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import threading

import dist
from dist import NAME, ROOT

WINDOWS = sys.platform == "win32"
BINARY = NAME + (".exe" if WINDOWS else "")


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args) -> None:
        pass


def manifest_version() -> str:
    text = (ROOT / "herdr-plugin.toml").read_text()
    return re.search(r'^version\s*=\s*"([^"]*)"', text, re.M).group(1)


def build_host_asset(into: pathlib.Path) -> pathlib.Path:
    goos = {"linux": "linux", "darwin": "darwin", "win32": "windows"}[sys.platform]
    goarch = "arm64" if platform.machine().lower() in ("arm64", "aarch64") else "amd64"
    return into / dist.build(into, goos, goarch)


def publish(releases: pathlib.Path, where: str, asset: pathlib.Path, tamper: bool = False) -> None:
    """Lay a release out under releases/where the way GitHub serves one."""
    directory = releases / where
    directory.mkdir(parents=True)
    shutil.copy(asset, directory / asset.name)
    digest = hashlib.sha256(asset.read_bytes()).hexdigest()
    if tamper:
        digest = "0" * 64
    (directory / "SHA256SUMS").write_text(f"{digest}  {asset.name}\n")


def plugin_copy(into: pathlib.Path) -> pathlib.Path:
    root = into / "plugin"
    skipped = {".git", "dist", NAME, NAME + ".exe"}

    def ignore(directory: str, names: list) -> set:
        return skipped & set(names) if pathlib.Path(directory) == ROOT else set()

    shutil.copytree(ROOT, root, ignore=ignore)
    return root


def without_go(work: pathlib.Path) -> str:
    """A PATH holding the system's own tools and no Go toolchain."""
    if WINDOWS:
        system = os.environ.get("SystemRoot", r"C:\Windows")
        return os.pathsep.join([system + r"\System32", system, system + r"\System32\WindowsPowerShell\v1.0"])
    # Some runners keep Go in /usr/bin, so the system tools are linked one by one.
    tools = work / "tools"
    tools.mkdir(exist_ok=True)
    for directory in ("/usr/bin", "/bin", "/usr/sbin", "/sbin"):
        for tool in pathlib.Path(directory).iterdir():
            link = tools / tool.name
            if tool.name not in ("go", "gofmt") and not link.exists():
                link.symlink_to(tool)
    return str(tools)


def failing_go(work: pathlib.Path, path: str) -> str:
    """PATH with a go in front of it that fails, as one too old to build would."""
    fake = work / "failing-go"
    fake.mkdir(exist_ok=True)
    if WINDOWS:
        (fake / "go.cmd").write_text("@exit /b 1\r\n")
    else:
        script = fake / "go"
        script.write_text("#!/bin/sh\nexit 1\n")
        script.chmod(0o755)
    return os.pathsep.join([str(fake), path])


def install(root: pathlib.Path, base: str, path: str) -> subprocess.CompletedProcess:
    if WINDOWS:
        command = ["powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "scripts/install.ps1"]
    else:
        command = ["sh", "scripts/install.sh"]
    env = dict(os.environ, HERDR_AUTO_TITLE_RELEASES=base, PATH=path)
    env.pop("HERDR_AUTO_TITLE_BUILD", None)
    result = subprocess.run(command, cwd=root, env=env, capture_output=True, text=True)
    sys.stderr.write(result.stderr)
    return result


def reported(root: pathlib.Path) -> str:
    binary = root / BINARY
    if not binary.exists():
        return ""
    return subprocess.run([str(binary), "--version"], capture_output=True, text=True).stdout.strip()


def main() -> None:
    version = manifest_version()
    work = pathlib.Path(tempfile.mkdtemp(prefix=NAME + "-test-"))
    asset = build_host_asset(work)

    releases = work / "releases"
    releases.mkdir()
    handler = functools.partial(QuietHandler, directory=str(releases))
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_address[1]}"

    failures = []

    def case(title: str, layout: list, path: str, installs: bool, needle: str) -> None:
        print(f"--- {title}", file=sys.stderr)
        shutil.rmtree(releases)
        releases.mkdir()
        for where, tamper in layout:
            publish(releases, where, asset, tamper)
        with tempfile.TemporaryDirectory(dir=work) as scratch:
            root = plugin_copy(pathlib.Path(scratch))
            result = install(root, base, path)
            got = reported(root)
            want = (0, full) if installs else (1, "")
            if (result.returncode, got) != want or needle not in result.stderr:
                failures.append(f"{title}: exit {result.returncode}, binary reports {got!r}")

    exact = f"download/v{version}"
    full = f"{NAME} {version}"
    go_path = os.environ["PATH"]
    no_go = without_go(work)
    latest = ("latest/download", False)
    case("this version's release, no Go", [(exact, False)], no_go, True, f"installed release {version}")
    case("release not out yet, no Go", [latest], no_go, True, "the latest published one")
    case("release not out yet, Go", [], go_path, True, "building from source")
    case("checksum mismatch, no Go", [(exact, True)], no_go, False, "which is not installed")
    case("no release, Go too old", [], failing_go(work, no_go), False, "the source build above failed")

    server.shutdown()
    shutil.rmtree(work, ignore_errors=True)
    for failure in failures:
        print(f"FAIL {failure}", file=sys.stderr)
    if failures:
        sys.exit(1)
    print("all install cases passed", file=sys.stderr)


if __name__ == "__main__":
    main()
