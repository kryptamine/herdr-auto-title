#!/usr/bin/env python3
"""Run this platform's install script against releases served locally.

Each case installs a copy of the plugin and checks what ended up in its root.
The binary served is the one scripts/dist.py builds, so the two agree on names.
"""

from __future__ import annotations

import functools
import hashlib
import http.server
import io
import json
import os
import pathlib
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import threading
from typing import Any, BinaryIO

import dist
from dist import NAME, ROOT

WINDOWS = sys.platform == "win32"
BINARY = NAME + (".exe" if WINDOWS else "")


class QuietHandler(http.server.SimpleHTTPRequestHandler):
    """Serves the releases, and a server error for any file beside a BROKEN one."""

    def send_head(self) -> io.BytesIO | BinaryIO | None:
        if (pathlib.Path(self.translate_path(self.path)).parent / "BROKEN").exists():
            self.send_error(500)
            return None
        return super().send_head()

    def log_message(self, format: str, *args: Any) -> None:
        pass


def manifest_version() -> str:
    text = (ROOT / "herdr-plugin.toml").read_text()
    found = re.search(r'^version\s*=\s*"([^"]*)"', text, re.M)
    if found is None:
        raise SystemExit("herdr-plugin.toml names no version")
    return found.group(1)


def manifest_build() -> list[str]:
    """The command Herdr runs for this platform, so the manifest is what is tested."""
    system = {"linux": "linux", "darwin": "macos", "win32": "windows"}[sys.platform]
    text = (ROOT / "herdr-plugin.toml").read_text()
    # A regex because tomllib needs Python 3.11, and macOS's python3 is 3.9.
    builds = re.findall(r"^\[\[build\]\]\nplatforms = \[(.*)\]\ncommand = \[(.*)\]$", text, re.M)
    command = next(command for platforms, command in builds if f'"{system}"' in platforms)
    arguments: list[str] = json.loads(f"[{command}]")
    return arguments


def build_host_asset(into: pathlib.Path) -> pathlib.Path:
    goos = {"linux": "linux", "darwin": "darwin", "win32": "windows"}[sys.platform]
    goarch = "arm64" if platform.machine().lower() in ("arm64", "aarch64") else "amd64"
    return into / dist.build(into, goos, goarch)


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


def plugin_copy(into: pathlib.Path, version: str) -> tuple[pathlib.Path, str]:
    """A checkout of the plugin at version, as a commit of its own, and that commit."""
    root = into / "plugin with spaces"
    skipped = {".git", ".mypy_cache", ".ruff_cache", ".venv", "dist", NAME, NAME + ".exe"}

    def ignore(directory: str, names: list[str]) -> set[str]:
        return skipped & set(names) if pathlib.Path(directory) == ROOT else set()

    shutil.copytree(ROOT, root, ignore=ignore)
    manifest = root / "herdr-plugin.toml"
    text = manifest.read_text()
    manifest.write_text(re.sub(r'^version\s*=\s*"[^"]*"', f'version = "{version}"', text, count=1, flags=re.M))

    def git(*args: str) -> str:
        identity = ["-c", "user.name=test", "-c", "user.email=test@localhost", "-c", "commit.gpgsign=false"]
        result = subprocess.run(["git", *identity, *args], cwd=root, capture_output=True, text=True, check=True)
        return result.stdout.strip()

    git("init", "-q")
    git("add", "-A")
    git("commit", "-q", "-m", "checkout")
    if WINDOWS:
        git("-c", "core.autocrlf=true", "checkout-index", "--force", "--all")
        assert b"\r" not in (root / "scripts" / "install.sh").read_bytes()
        assert b"\r\n" in (root / "scripts" / "install.cmd").read_bytes()
    return root, git("rev-parse", "HEAD")


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


def failing_go(work: pathlib.Path, path: str) -> str:
    """PATH with a go in front of it that fails, as one too old to build would."""
    fake = work / "failing-go"
    fake.mkdir(exist_ok=True)
    script = fake / "go"
    script.write_bytes(b"#!/bin/sh\nexit 1\n")
    script.chmod(0o755)
    return os.pathsep.join([str(fake), path])


def install(root: pathlib.Path, base: str, path: str) -> subprocess.CompletedProcess[str]:
    env = dict(os.environ, HERDR_AUTO_TITLE_RELEASES=base, PATH=path)
    result = subprocess.run(manifest_build(), cwd=root, env=env, capture_output=True, text=True)
    sys.stderr.write(result.stderr)
    return result


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


def reported(root: pathlib.Path) -> str:
    binary = root / BINARY
    if not binary.exists():
        return ""
    return subprocess.run([str(binary), "--version"], capture_output=True, text=True).stdout.strip()


def main() -> None:
    version = manifest_version()
    work = pathlib.Path(tempfile.mkdtemp(prefix=NAME + "-test-"))
    asset = build_host_asset(work)
    garbage = work / "garbage" / asset.name
    garbage.parent.mkdir()
    garbage.write_bytes(b"not a program\n")

    releases = work / "releases"
    releases.mkdir()
    handler = functools.partial(QuietHandler, directory=str(releases))
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_address[1]}"

    failures: list[str] = []

    def case(title: str, path: str, installs: bool, needle: str, **release: Any) -> None:
        """Install a fresh checkout against one release, laid out as the keywords say."""
        print(f"--- {title}", file=sys.stderr)
        shutil.rmtree(releases)
        releases.mkdir()
        with tempfile.TemporaryDirectory(dir=work) as scratch:
            root, head = plugin_copy(pathlib.Path(scratch), release.get("checkout", version))
            if release.get("bad_source"):
                (root / "go.mod").write_text("invalid module file\n")
            commit = head if release.get("released", True) else "0" * 40
            for where in release.get("at", [f"download/v{release.get('checkout', version)}"]):
                publish(
                    releases,
                    where,
                    release.get("serve", asset),
                    commit,
                    release.get("tamper", False),
                    release.get("broken", False),
                )
            if release.get("latest"):
                # One that would pass every check: only a fallback to latest can take it.
                publish(releases, "latest/download", asset, head)
            result = install(root, base, path)
            got = reported(root)
            if WINDOWS and installs and (root / BINARY).exists():
                data = (root / BINARY).read_bytes()
                pe = int.from_bytes(data[60:64], "little")
                machine = int.from_bytes(data[pe + 4 : pe + 6], "little")
                expected = 0xAA64 if "arm64" in asset.name else 0x8664
                if machine != expected:
                    failures.append(f"{title}: PE machine {machine:#x}, expected {expected:#x}")
            want = (0, f"{NAME} {version}") if installs else (1, "")
            unwanted = release.get("absent")
            said = needle in result.stderr and not (unwanted and unwanted in result.stderr)
            if (result.returncode, got) != want or not said:
                failures.append(f"{title}: exit {result.returncode}, binary reports {got!r}")

    go_path = os.environ["PATH"]
    no_go = without_go(work)
    case("this release, no Go", no_go, True, f"downloading {asset.name}")
    case("this release, Go", go_path, True, f"installed release {version}")
    case("main after a release, latest answers, no Go", no_go, False, "is not release", released=False, latest=True)
    case("this release not out, latest answers, no Go", no_go, False, "could not be fetched", at=[], latest=True)
    case("this release fails, latest answers, no Go", no_go, False, "could not be fetched", broken=True, latest=True)
    case("this release mismatches, latest answers, no Go", no_go, False, "does not match", tamper=True, latest=True)
    case("not this release, Go", go_path, True, "building from source", released=False)
    case("not this release, no Go", no_go, False, "without Go", released=False)
    case(
        "not this release, Go too old", failing_go(work, no_go), False, "the source build above failed", released=False
    )
    case("checksum mismatch, no Go", no_go, False, "does not match its checksum", tamper=True)
    case("checksum mismatch, Go", go_path, True, "building from source", tamper=True)
    case("binary of another version, no Go", no_go, False, "not 0.0.1", checkout="0.0.1")
    case("binary that does not run, no Go", no_go, False, "does not run", serve=garbage)
    if WINDOWS:
        system = os.environ.get("SYSTEMROOT", r"C:\Windows")
        no_bash = os.pathsep.join([system + r"\System32", system])
        shim_path = os.pathsep.join([str(git_shim(work)), no_bash])
        case("Git shim, no Go", shim_path, True, f"installed release {version}")
        go = shutil.which("go")
        assert go is not None
        with_go = os.pathsep.join([no_bash, str(pathlib.Path(go).parent)])
        case("no Bash, no Go", no_bash, False, "install Git for Windows or Go")
        case("no Bash, Go", with_go, True, "Git Bash is unavailable")
        case(
            "no Bash, failing build",
            with_go,
            False,
            "it could not be built from source, which needs Go 1.24 or newer",
            bad_source=True,
        )
        incomplete = work / "incomplete Git"
        for name in ("cmd/git.exe", "bin/bash.exe", "usr/bin/uname.exe"):
            target = incomplete / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(pathlib.Path(system) / "System32" / "where.exe", target)
        unusable = os.pathsep.join([str(incomplete / "cmd"), no_bash])
        case("unusable Git Bash, no Go", unusable, False, "install Git for Windows or Go")
    if not WINDOWS and shutil.which("wget"):
        case("this release over wget, no Go", without_go(work, ("curl",)), True, f"installed release {version}")
    if not WINDOWS:
        no_fetch = without_go(work, ("curl", "wget"))
        case("no curl or wget, no Go", no_fetch, False, "neither curl nor wget", absent="--ref stable")

    server.shutdown()
    shutil.rmtree(work, ignore_errors=True)
    for failure in failures:
        print(f"FAIL {failure}", file=sys.stderr)
    if failures:
        sys.exit(1)
    print("all install cases passed", file=sys.stderr)


if __name__ == "__main__":
    main()
