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
    """Serves the releases, and a server error for any file beside a BROKEN one."""

    def send_head(self):
        if (pathlib.Path(self.translate_path(self.path)).parent / "BROKEN").exists():
            self.send_error(500)
            return None
        return super().send_head()

    def log_message(self, *args) -> None:
        pass


def manifest_version() -> str:
    text = (ROOT / "herdr-plugin.toml").read_text()
    return re.search(r'^version\s*=\s*"([^"]*)"', text, re.M).group(1)


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


def plugin_copy(into: pathlib.Path, version: str) -> tuple:
    """A checkout of the plugin at version, as a commit of its own, and that commit."""
    root = into / "plugin"
    skipped = {".git", "dist", NAME, NAME + ".exe"}

    def ignore(directory: str, names: list) -> set:
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
    return root, git("rev-parse", "HEAD")


def without_go(work: pathlib.Path, also: tuple = ()) -> str:
    """A PATH holding the system's own tools and git, and no Go toolchain."""
    if WINDOWS:
        system = os.environ.get("SystemRoot", r"C:\Windows")
        git = os.path.dirname(shutil.which("git"))
        return os.pathsep.join([system + r"\System32", system, system + r"\System32\WindowsPowerShell\v1.0", git])
    # Some runners keep Go in /usr/bin, so the system tools are linked one by one.
    tools = work / ("tools-without-" + "-".join(("go",) + also))
    tools.mkdir(exist_ok=True)
    hidden = {"go", "gofmt", *also}
    for directory in ("/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/local/bin", "/opt/homebrew/bin"):
        if not pathlib.Path(directory).is_dir():
            continue
        for tool in pathlib.Path(directory).iterdir():
            link = tools / tool.name
            if tool.name not in hidden and not link.exists():
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
    garbage = work / "garbage" / asset.name
    garbage.parent.mkdir()
    garbage.write_bytes(b"not a program\n")

    releases = work / "releases"
    releases.mkdir()
    handler = functools.partial(QuietHandler, directory=str(releases))
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    base = f"http://127.0.0.1:{server.server_address[1]}"

    failures = []

    def case(title: str, path: str, installs: bool, needle: str, **release) -> None:
        """Install a fresh checkout against one release, laid out as the keywords say."""
        print(f"--- {title}", file=sys.stderr)
        shutil.rmtree(releases)
        releases.mkdir()
        with tempfile.TemporaryDirectory(dir=work) as scratch:
            root, head = plugin_copy(pathlib.Path(scratch), release.get("checkout", version))
            commit = head if release.get("released", True) else "0" * 40
            for where in release.get("at", [f"download/v{release.get('checkout', version)}"]):
                publish(
                    releases, where, release.get("serve", asset), commit, release.get("tamper", False),
                    release.get("broken", False),
                )
            if release.get("latest"):
                # One that would pass every check: only a fallback to latest can take it.
                publish(releases, "latest/download", asset, head)
            result = install(root, base, path)
            got = reported(root)
            want = (0, f"{NAME} {version}") if installs else (1, "")
            unwanted = release.get("absent")
            said = needle in result.stderr and not (unwanted and unwanted in result.stderr)
            if (result.returncode, got) != want or not said:
                failures.append(f"{title}: exit {result.returncode}, binary reports {got!r}")

    go_path = os.environ["PATH"]
    no_go = without_go(work)
    case("this release, no Go", no_go, True, f"installed release {version}")
    case("this release, Go", go_path, True, f"installed release {version}")
    case("main after a release, latest answers, no Go", no_go, False, "is not release", released=False, latest=True)
    case("this release not out, latest answers, no Go", no_go, False, "could not be fetched", at=[], latest=True)
    case("this release fails, latest answers, no Go", no_go, False, "could not be fetched", broken=True, latest=True)
    case("this release mismatches, latest answers, no Go", no_go, False, "does not match", tamper=True, latest=True)
    case("not this release, Go", go_path, True, "building from source", released=False)
    case("not this release, no Go", no_go, False, "without Go", released=False)
    case("not this release, Go too old", failing_go(work, no_go), False, "the source build above failed", released=False)
    case("checksum mismatch, no Go", no_go, False, "does not match its checksum", tamper=True)
    case("checksum mismatch, Go", go_path, True, "building from source", tamper=True)
    case("binary of another version, no Go", no_go, False, "not 0.0.1", checkout="0.0.1")
    case("binary that does not run, no Go", no_go, False, "does not run", serve=garbage)
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
