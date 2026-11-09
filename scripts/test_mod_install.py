#!/usr/bin/env python3
"""Test gallery installers with local releases and a harmless native executable."""

from __future__ import annotations

import functools
import http.server
import os
import pathlib
import platform
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest

from dist import NAME, ROOT
from release_fixture import WINDOWS, QuietHandler, git_shim, publish, without_go

VERSION = "0.21.0"
MOD = ROOT / "mod" / "mods" / "auto-title"


class GalleryInstall(unittest.TestCase):
    """Exercise the same entry point the popup opens, without registering a service."""

    work: pathlib.Path
    scratch: tempfile.TemporaryDirectory[str]
    stub: pathlib.Path
    arch: str
    server: http.server.ThreadingHTTPServer
    thread: threading.Thread
    releases: pathlib.Path
    requests: list[str]
    routes: dict[str, str]

    @classmethod
    def setUpClass(cls) -> None:
        cls.scratch = tempfile.TemporaryDirectory(prefix="auto-title-gallery-")
        cls.addClassCleanup(cls.scratch.cleanup)
        cls.work = pathlib.Path(cls.scratch.name)
        cls.arch = "arm64" if platform.machine().lower() in ("arm64", "aarch64") else "amd64"
        cls.stub = cls.work / ("stub.exe" if WINDOWS else "stub")
        source = cls.work / "stub.go"
        source.write_text(
            'package main\nimport ("os"; "path/filepath"; "runtime"; "strings")\nfunc main() {\n'
            'f, err := os.OpenFile(os.Getenv("MOD_CALLED"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600); '
            "if err != nil { panic(err) }; defer f.Close()\n"
            "exe, err := os.Executable(); if err != nil { panic(err) }\n"
            '_, err = f.WriteString(runtime.GOARCH+"\\n"+filepath.Base(exe)+"\\n"+'
            'strings.Join(os.Args[1:], " ")+"\\n"); if err != nil { panic(err) }\n'
            'if os.Getenv("MOD_FAIL") == "1" { os.Exit(1) }\n}\n'
        )
        subprocess.run(
            ["go", "build", "-o", str(cls.stub), str(source)],
            env=dict(os.environ, GOARCH=cls.arch),
            check=True,
        )
        cls.releases = cls.work / "releases"
        cls.releases.mkdir()
        cls.requests = []

        class Handler(QuietHandler):
            def do_GET(self) -> None:
                cls.requests.append(self.path)
                super().do_GET()

        cls.server = http.server.ThreadingHTTPServer(
            ("127.0.0.1", 0), functools.partial(Handler, directory=str(cls.releases))
        )
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.addClassCleanup(cls.server.server_close)
        cls.addClassCleanup(cls.server.shutdown)
        if WINDOWS:
            system = os.environ.get("SYSTEMROOT", r"C:\Windows")
            native_path = os.pathsep.join([system + r"\System32", system])
            cls.routes = {"Git Bash": without_go(cls.work), "PowerShell": native_path}
            cls.routes["Git shim"] = os.pathsep.join([str(git_shim(cls.work)), native_path])
        else:
            cls.routes = {"sh": os.environ["PATH"]}

    def run_case(
        self,
        route: str,
        *,
        published: bool = True,
        tamper: bool = False,
        fails: bool = False,
        reject: bool = False,
        architecture: str | None = None,
        punctuation: bool = False,
    ) -> None:
        with tempfile.TemporaryDirectory(dir=self.work) as scratch:
            folder = pathlib.Path(scratch) / ("A&B" if punctuation else "paths ' & spaces \u00e9")
            installer = folder / "mods" / "auto-title"
            installer.mkdir(parents=True)
            for name in ("install.cmd", "install.sh", "install.ps1"):
                shutil.copyfile(MOD / name, installer / name)
            if reject:
                # Windows 11 on Arm runs no batch file under a fake SystemRoot, so fail in PowerShell.
                (installer / "install.ps1").write_text("param(\n", encoding="utf-8")
            data = folder / "data" / "auto-title-install-test"
            data.mkdir(parents=True)
            marker = folder / "called"
            system_name = {"win32": "windows", "darwin": "macos"}.get(sys.platform, "linux")
            asset_name = f"{NAME}-{system_name}-{architecture or self.arch}" + (".exe" if WINDOWS else "")
            asset = folder / asset_name
            shutil.copyfile(self.stub, asset)
            shutil.rmtree(self.releases)
            self.releases.mkdir()
            self.requests.clear()
            if published:
                publish(self.releases, f"download/v{VERSION}", asset, "0" * 40, tamper)
            env = dict(
                os.environ,
                HERDR_AUTO_TITLE_RELEASES=f"http://127.0.0.1:{self.server.server_address[1]}",
                MOD_CALLED=str(marker),
                MOD_FAIL="1" if fails else "0",
                PATH=self.routes[route],
            )
            env.pop("HC_TERMINAL_ID", None)
            if architecture:
                env.update(PROCESSOR_ARCHITECTURE="x86", PROCESSOR_ARCHITEW6432=architecture.upper())
            command = [os.environ["COMSPEC"], "/d", "/c", "install.cmd"] if WINDOWS else ["sh", "install.sh"]
            result = subprocess.run(
                [*command, VERSION, "../../data/auto-title-install-test" if WINDOWS else str(data)],
                cwd=installer,
                env=env,
                stdin=subprocess.DEVNULL,
                capture_output=True,
                timeout=45,
            )
            output = (result.stdout + result.stderr).decode("utf-8", errors="replace")
            status_file = data / "status"
            self.assertTrue(status_file.exists(), output)
            status = status_file.read_text(encoding="utf-8")
            success = published and not tamper and not fails and not reject
            self.assertEqual(result.returncode == 0, success, output)
            self.assertEqual(status.splitlines()[0], "0" if success else "1", output)
            if WINDOWS:
                expected_route = "PowerShell" if route == "PowerShell" else "Git Bash"
                self.assertIn(f"using {expected_route}".lower(), output.lower())
                if expected_route == "Git Bash":
                    self.assertNotIn("using PowerShell", output)
            should_run = published and not tamper and not reject
            self.assertEqual(marker.exists(), should_run, output)
            if should_run:
                self.assertEqual(marker.read_text(), f"{self.arch}\n{asset_name}\ndesktop install\n")
            if reject:
                self.assertIn("PowerShell could not run the installer", status)
                self.assertEqual(self.requests, [])
            else:
                self.assertIn(f"/download/v{VERSION}/{asset_name}", self.requests)
            if tamper:
                self.assertIn("does not match SHA256SUMS", status)
            if not published:
                self.assertIn("download", status)
            if fails:
                self.assertIn("desktop install failed", status)
            self.assertEqual(sorted(p.name for p in data.iterdir()), ["status"], output)

    def test_release_and_failures_on_every_route(self) -> None:
        for route in self.routes:
            for scenario in ("success", "missing", "checksum", "install failure"):
                with self.subTest(route=route, scenario=scenario):
                    self.run_case(
                        route,
                        published=scenario != "missing",
                        tamper=scenario == "checksum",
                        fails=scenario == "install failure",
                    )

    @unittest.skipUnless(WINDOWS, "CMD metacharacters without whitespace")
    def test_parent_path_with_unquoted_metacharacters(self) -> None:
        for route in self.routes:
            with self.subTest(route=route):
                self.run_case(route, punctuation=True)

    @unittest.skipUnless(WINDOWS, "Windows fallback startup")
    def test_fallback_startup_failure_reports_status(self) -> None:
        self.run_case("PowerShell", reject=True)

    @unittest.skipUnless(WINDOWS, "Windows native architecture from an emulated shell")
    def test_powershell_prefers_native_architecture(self) -> None:
        for arch in ("amd64", "arm64"):
            with self.subTest(architecture=arch):
                if self.arch == "arm64" and arch != "arm64":
                    self.skipTest("Windows 11 on Arm resets PROCESSOR_ARCHITECTURE for a native child")
                self.run_case("PowerShell", architecture=arch)


if __name__ == "__main__":
    unittest.main()
