# Windows mod execution: source research

Read and probed on 2026-11-09. Community examples were inspected only;
the runtime probes below used harmless local fixtures on Windows.
The [community gallery](https://herdr.dev/gallery/?type=mods) lists both
Vitals for Windows (FoxCreate) and Packages (manastalukdar). Their approaches
differ; these examples do not establish a universal mod installation pattern.

## Vitals for Windows

Inspected commit `4813fd1925db5e9b994852871c9ab3c5b4057eb5`.

- Declared support: Windows 10/11 with PowerShell 7 or Windows PowerShell 5.1.
  Its README explicitly warns: "A work PC whose IT policy blocks PowerShell
  scripts will block the sampler."
  [README](https://github.com/FoxCreate/herdr-vitals-windows/blob/4813fd1925db5e9b994852871c9ab3c5b4057eb5/README.md)
- Verified code: `winStart()` invokes `pwsh`, falling back on a rejected launch
  to `powershell`, with `-NoProfile -NonInteractive -Command`. That command
  creates the data directory and starts another PowerShell process with
  `-ExecutionPolicy Bypass -File` pointing at `sampler.ps1`.
  This retains a script-file dependency; it does not demonstrate compatibility
  with policy that blocks script files.
  [main.js](https://github.com/FoxCreate/herdr-vitals-windows/blob/4813fd1925db5e9b994852871c9ab3c5b4057eb5/mods/vitals/main.js)
- Verified code: readings and heartbeat use `readTextFile`/`writeTextFile`.
  The updater fetches text files from a version tag, validates the manifest's
  filenames, preserves locally changed files as `.mine`, and writes `main.js`
  last. It is text-file updating, not a native Windows binary installer.
  [main.js](https://github.com/FoxCreate/herdr-vitals-windows/blob/4813fd1925db5e9b994852871c9ab3c5b4057eb5/mods/vitals/main.js)
- Binary provisioning in this mod belongs to its Unix machine path: `sh`
  downloads the original herdr-vitals release and checks its checksum. The
  local Windows path uses the PowerShell sampler instead.
  [main.js](https://github.com/FoxCreate/herdr-vitals-windows/blob/4813fd1925db5e9b994852871c9ab3c5b4057eb5/mods/vitals/main.js)

## Packages

Inspected commit `0fad8c14aed0a0f076b84bfc6f6e9f94c2f9baa4`.

- Declared support is qualified: the author reports a Linux-tested dialog,
  a Linux/macOS/Windows CI matrix, and Python plus Git prerequisites. This is
  not a claim that the Windows dialog was exercised on a real machine.
  [README](https://github.com/manastalukdar/herdr-pkg/blob/0fad8c14aed0a0f076b84bfc6f6e9f94c2f9baa4/README.md)
- Verified code: the Windows branch tries `exec("py", ["-3", helper, ...])`,
  then `python` if launching rejects. It passes `pkg.py` and JSON-operation
  arguments directly as argv, checks the exit code, and parses stdout as JSON.
  Opening a repository uses `explorer` on Windows. This avoids a PowerShell
  entry point but requires an installed Python interpreter.
  [main.js](https://github.com/manastalukdar/herdr-pkg/blob/0fad8c14aed0a0f076b84bfc6f6e9f94c2f9baa4/mods/pkg/main.js)

## First-party comparison

Inspected installed Herdr app 0.1.14 and upstream commit
`b721deda985270ec8256552692729a4add389b2a`:

- Files uses `powershell -NoProfile -Command` for Windows directory listing,
  and `readTextFile` for previews.
  [files/main.js](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/defaults/mods/files/main.js)
- Worktree executes Git with argv and uses a small PowerShell `-Command`
  expression for remote Windows home discovery.
  [worktree/main.js](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/defaults/mods/worktree/main.js)
- The API exposes native text reads/writes and an execution timeout option:
  30 seconds by default, at most 600 seconds. A larger timeout alone says
  nothing about a popup's lifetime or cancellation behavior.
  [herdr.d.ts](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/defaults/herdr.d.ts)

## Decision boundary

These sources establish three available mechanisms: native text operations,
direct argv execution of an installed program, and explicit shell commands.
They do not establish a policy-compatible, dependency-free Windows binary
download/install replacement. In particular, Vitals is a counterexample to
assuming that an outer `-Command` removes an inner `-File` dependency.

No community mod was installed or executed. No administrative policy was
changed. The following probes narrow the implementation choices; they do not
validate a replacement installer or install a real login service.

## Runtime probes: Herdr 0.1.14 on Windows

Test host: Windows build 26200, herdr-app 0.1.14, hc 0.1.19, amd64.
A temporary popup ran an executable built locally that only wrote start/end
markers. Every popup was removed; the original focus, tabs and error-free app
state were read back. Probe artifacts are in
`%TEMP%/auto-title-pr212-lifecycle/`.

### Execution lifetime

- A 32-second child completed successfully with `{timeout: 35000}`; measured
  elapsed time was 32,061 ms. This option requires 0.1.14: the
  [0.1.13 API](https://github.com/herdrdev/herdr-app/blob/v0.1.13/defaults/herdr.d.ts)
  lacks it, while the
  [0.1.14 API](https://github.com/herdrdev/herdr-app/blob/v0.1.14/defaults/herdr.d.ts)
  includes it.
- Closing the popup after a five-second child started left the child running:
  its end marker appeared, but the JavaScript continuation's marker did not.
- A 500 ms timeout returned an error after 510 ms and stopped the child; its
  end marker did not appear after its original five-second duration.
- This matches the source: `exec_future` starts an independent thread;
  `run_blocking` owns and times out the child, independently of popup lifetime.
  [Implementation](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/src/mods.rs#L753).

Increasing the timeout therefore does not make a download/hash/install chain
survive closing the popup. Only the currently started operation survives.
An independent process owning the entire transaction is still useful.

### Native files, download and hashing

- `fs.mkdir` under `%TEMP%` was refused by the capability policy. Creating a
  directory under `appHome()/data` worked. The app grants native reads under
  the user's home and native writes under the app home, so this is the intended
  location rather than a reason to broaden capabilities.
  [Policy](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/src/mods.rs#L1143).
- Direct `exec` of `C:/Windows/System32/curl.exe` downloaded a 2,257,920-byte
  fixture from local HTTP into a path with spaces, Unicode and shell punctuation.
  An independent Python SHA256 matched the original executable exactly.
  This verifies the local transfer, not GitHub/TLS/proxy behavior.
- `fs.readFile` returned a `Uint8Array` with the right length. However,
  `createHash("sha256").update(bytes).digest("hex")` returned the SHA256 of
  empty input, both for that executable and for a file containing `abc`.
  Hashing the string `"abc"` returned the correct `ba7816bf...15ad` digest.
  This runtime path is unsuitable for release verification until fixed or
  explained; no downloaded executable was run after the mismatch.
- Source intends byte hashing to work: LLRT's
  [hash implementation](https://github.com/awslabs/llrt/blob/7b95c82a9b15e7ddfb2778eca4b5a63111e74f51/modules/llrt_crypto/src/hash.rs)
  uses [ObjectBytes](https://github.com/awslabs/llrt/blob/7b95c82a9b15e7ddfb2778eca4b5a63111e74f51/libs/llrt_utils/src/bytes.rs),
  which explicitly handles typed byte arrays. The runtime defect's root cause
  has not been established.
- Native `fetch` is not an alternative for release binaries: its response
  body is converted to lossy UTF-8 and exposes text/JSON only.
  [Source](https://github.com/herdrdev/herdr-app/blob/b721deda985270ec8256552692729a4add389b2a/vendor/gpui-kit/crates/shell/src/engine/quickjs/standard/fetch.rs#L267).

### PowerShell distinction

A fixed .NET SHA256 expression executed with `powershell.exe -NoProfile
-ExecutionPolicy Restricted -Command ...` reported `Restricted` and correctly
hashed the `abc` fixture. No registry policy was changed. This tests individual
commands under a process-level policy, not a corporate GPO/AppLocker/WDAC setup.
Microsoft documents that Restricted permits individual commands while blocking
script files, and that process policy cannot override Group Policy.
[Execution policy](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_execution_policies?view=powershell-5.1).

The existing Windows service implementation already invokes its embedded
Task Scheduler logic using `-EncodedCommand`, not a `.ps1` file
(`internal/autostart/scheduler.go`). Removing `.ps1` therefore does not imply
eliminating all PowerShell use, nor does it establish compatibility with every
enterprise application-control policy.

`certutil -hashfile` exists, but Microsoft explicitly discourages production
use of certutil and makes no application-compatibility guarantees. It is not
the preferred foundation for a new installer.
[Certutil](https://learn.microsoft.com/en-us/windows-server/administration/windows-commands/certutil).

## Selected direction

Keep one independent terminal process responsible for download, checksum
verification and `desktop install`, so closing the popup cannot abandon the
transaction. `install.cmd` reuses the existing Git Bash discovery pattern and
runs the shared shell installer. When no usable Bash is found, retain
`install.ps1` as the fallback. A Bash installation failure must not retry the
transaction through PowerShell.

Use native file APIs for staging and status, under `appHome()/data`; keep the
fixed `-Command` environment lookup separate from the `.ps1` fallback. The
fallback remains subject to execution policy, and CMD reports startup refusal.
The native JavaScript hash failure makes a pure-JavaScript replacement unsuitable.

CI runs installer fixtures on Linux, macOS and Windows, each on x64 and ARM64.
Windows fixtures exercise both routes, Git shims, failed startup and failed
transactions. They use harmless binaries and do not register login services.
