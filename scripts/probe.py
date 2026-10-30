#!/usr/bin/env python3
"""Inspect the live Herdr socket.

Verify against the running Herdr before writing code that assumes anything.
"""

import contextlib
import json
import os
import socket
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

USAGE = """usage: scripts/probe.py <command>

  tabs         one-shot list of tab ids and labels
  watch-tabs   the same, refreshed every second
  snapshot     the session snapshot Auto Title polls
  workspaces   each workspace's label, active tab and reported tokens
  version      protocol and version of the running Herdr
  desktop      the desktop app's tabs, joined to herdr-core's terminals
"""


def herdr(*args: str) -> dict[str, Any]:
    """Run a herdr CLI command and return its parsed JSON result."""
    out = subprocess.run(["herdr", *args], capture_output=True, text=True, check=True).stdout
    payload = json.loads(out)
    if "error" in payload:
        raise SystemExit(f"herdr {' '.join(args)}: {payload['error']}")
    result: dict[str, Any] = payload["result"]
    return result


def cmd_tabs() -> None:
    for tab in herdr("tab", "list")["tabs"]:
        print(f"{tab['tab_id']:10} {tab['agent_status']:8} {tab['label']}")


def cmd_watch_tabs() -> None:
    while True:
        print("\033[2J\033[H", end="")
        print(time.strftime("%H:%M:%S"))
        cmd_tabs()
        sys.stdout.flush()
        time.sleep(1)


def cmd_snapshot() -> None:
    snap = herdr("api", "snapshot")["snapshot"]
    print(f"herdr {snap['version']}, protocol {snap['protocol']}")
    print("\ntabs")
    for tab in snap["tabs"]:
        print(f"  {tab['tab_id']:10} panes={tab['pane_count']} label={tab['label']!r}")
    print("\npanes")
    for pane in snap["panes"]:
        agent = pane.get("agent") or "-"
        print(f"  {pane['pane_id']:10} tab={pane['tab_id']:10} agent={agent:8} cwd={pane.get('cwd')}")
        title = pane.get("terminal_title_stripped")
        if title:
            print(f"  {'':10} title={title!r}")


def cmd_workspaces() -> None:
    snap = herdr("api", "snapshot")["snapshot"]
    for ws in snap["workspaces"]:
        print(
            f"{ws['workspace_id']:6} active={ws.get('active_tab_id') or '-':8} "
            f"tabs={ws.get('tab_count')} label={ws['label']!r} tokens={ws.get('tokens') or {}}"
        )


def cmd_version() -> None:
    snap = herdr("api", "snapshot")["snapshot"]
    print(f"herdr {snap['version']}, protocol {snap['protocol']}")


def ask(path: Path, payload: dict[str, Any]) -> dict[str, Any]:
    """Send one JSON line to a Unix socket and return the line it answers."""
    with socket.socket(socket.AF_UNIX) as conn:
        conn.settimeout(6)
        conn.connect(str(path))
        conn.sendall((json.dumps(payload) + "\n").encode())
        reply = b""
        while not reply.endswith(b"\n"):
            chunk = conn.recv(65536)
            if not chunk:
                break
            reply += chunk
    answer: dict[str, Any] = json.loads(reply)
    return answer


def core_socket() -> Path:
    """Where herdr-core listens, by the rules in its docs/security.md."""
    runtime = os.environ.get("XDG_RUNTIME_DIR")
    if not runtime and Path(f"/run/user/{os.getuid()}").exists():
        runtime = f"/run/user/{os.getuid()}"
    if runtime:
        return Path(runtime) / "herdr-core" / "default.sock"
    state = os.environ.get("XDG_STATE_HOME") or Path.home() / ".local" / "state"
    return Path(state) / "herdr-core" / "default" / "default.sock"


def app_socket() -> Path:
    home = os.environ.get("HERDR_APP_HOME") or Path.home() / ".config" / "herdr-app"
    return Path(home) / "herdr-app.sock"


def core_terminals() -> dict[str, dict[str, Any]]:
    """herdr-core's terminals, keyed by the address the app's tabs name them by."""
    request = {"jsonrpc": "2.0", "id": 1, "method": "terminal.list", "params": {}}
    terminals = ask(core_socket(), request)["result"]["terminals"]
    return {terminal["address"]: terminal for terminal in terminals}


def print_terminal(entry: dict[str, Any], terminal: dict[str, Any]) -> None:
    foreground = terminal.get("foreground") or {}
    print(
        f"    {entry['terminal']:6} pane={entry['pane']} "
        f"status={entry['status']} app={entry['app']} "
        f"pid={terminal.get('pid')} foreground={foreground.get('name')}"
    )
    print(f"    {'':6} title={terminal.get('title')!r}")


def cmd_desktop() -> None:
    app = app_socket()
    terminals = core_terminals()
    print(f"focused pane {ask(app, {'status': True}).get('focused_pane')}")
    for space in ask(app, {"ls": True})["spaces"]:
        print(f"\n{space['id']} {space['name']!r} machine={space['machine']}")
        for tab in space["tabs"]:
            print(f"  {tab['id']} name={tab['name']!r} active={tab['active']}")
            for entry in tab["terminals"]:
                print_terminal(entry, terminals.get(entry["terminal"], {}))


COMMANDS = {
    "desktop": cmd_desktop,
    "tabs": cmd_tabs,
    "watch-tabs": cmd_watch_tabs,
    "snapshot": cmd_snapshot,
    "workspaces": cmd_workspaces,
    "version": cmd_version,
}


def main() -> int:
    if len(sys.argv) != 2 or sys.argv[1] not in COMMANDS:
        print(USAGE, file=sys.stderr)
        return 1
    with contextlib.suppress(KeyboardInterrupt):
        COMMANDS[sys.argv[1]]()
    return 0


if __name__ == "__main__":
    sys.exit(main())
