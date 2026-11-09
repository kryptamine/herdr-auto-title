// What this machine has of Auto Title, and how it gets the release this mod
// belongs to. The binary owns the service: it sets it up, takes it away and
// says what state it is in (`desktop status`); this only asks it.

import { platform } from "os";
import { randomUUID } from "crypto";
import { exists, mkdir, readFile, readdir, unlink, rmdir } from "fs/promises";
import { appHome, exec, home, run } from "herdr";

/** @typedef {{ version: string, running: boolean, latest?: string }} Copy
 *  latest: a newer release the service found since this mod's */
/** @typedef {(text: string) => void} Step */
/** @typedef {(ms: number) => Promise<void>} Sleep */

// VERSION is the release this mod installs. release-please keeps it equal to
// herdr-plugin.toml, and a Go test holds the two together.
export const VERSION = "0.21.0"; // x-release-please-version

const NAME = "herdr-auto-title";
const POLL_MS = 1000;
const INSTALL_TIMEOUT_MS = 10 * 60 * 1000;

// A renamed mod must change this path to its scripts too.
const MOD = `${appHome()}/mods/auto-title`;

// COPIES says where `desktop install` keeps its copy on each OS, as
// internal/autostart does. An OS missing here is not supported yet.
/** @type {Record<string, () => Promise<string>>} */
const COPIES = {
  macos: () => Promise.resolve(`${home()}/Library/Application Support/${NAME}/bin/${NAME}`),
  linux: async () => {
    const data = await output("sh", ["-c", 'printf %s "${XDG_DATA_HOME:-$HOME/.local/share}"']);

    return `${data}/${NAME}/${NAME}`;
  },
  windows: async () => (await output("powershell.exe", ["-NoProfile", "-Command",
    "[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false); " +
    "$root = $env:LOCALAPPDATA; " +
    "if (-not $root -or $root.Replace('\\', '/') -notmatch '^(?:[a-z]:/|//[^/]+/[^/]+)') { " +
    "$root = Join-Path ([Environment]::GetFolderPath('UserProfile')) 'AppData/Local' }; " +
    "$copy = Join-Path $root 'herdr-auto-title/bin/herdr-auto-title.exe'; " +
    "if (Test-Path -LiteralPath $copy -PathType Leaf) { [Console]::Write($copy) }",
  ])).trim(),
};

const os = { darwin: "macos", win32: "windows" }[platform()] ?? platform();
const where = COPIES[os];

export const supported = where !== undefined;

// unsupported is what the popup says on an OS with no copy to ask.
export const unsupported = `Not available on ${{ windows: "Windows" }[os] ?? os} yet.`;

/** copy is the installed copy's path; the strip asks it of no OS it does not support.
 *  Windows returns an empty path when no copy exists. @returns {Promise<string>} */
function copy() {
  if (!where) {
    throw new Error(unsupported);
  }

  return where();
}

/** output is a program's stdout, or an error naming the step and what it said.
 *  @param {string} program @param {string[]} args @param {string} [step] @returns {Promise<string>} */
async function output(program, args, step = program) {
  const { code, stdout, stderr } = await exec(program, args);

  if (code !== 0) {
    const said = (stderr || stdout).trim() || `exit ${String(code)}`;

    throw new Error(`${step}: ${said}`);
  }

  return stdout;
}

/** installed is what the copy says of itself, or null when there is no copy.
 *  A copy that cannot answer is an error, never "not installed".
 *  @returns {Promise<Copy | null>} */
export async function installed() {
  const path = await copy();

  if (!path) {
    return null;
  }

  // Native file reads cannot reach an XDG directory outside the user's home.
  if (os !== "windows") {
    const { code } = await exec("test", ["-x", path]);

    if (code !== 0) {
      return null;
    }
  }

  return parseCopy(await output(path, ["desktop", "status"], "desktop status"));
}

/** @param {string} json @returns {Copy} */
function parseCopy(json) {
  /** @type {unknown} */
  const value = JSON.parse(json);

  if (
    typeof value === "object" &&
    value !== null &&
    "version" in value &&
    typeof value.version === "string" &&
    "running" in value &&
    typeof value.running === "boolean"
  ) {
    const latest = "latest" in value && typeof value.latest === "string" ? value.latest : undefined;

    return { version: value.version, running: value.running, latest };
  }

  throw new Error(`desktop status: not a status: ${json.trim()}`);
}

/** older is whether version a comes before version b, part by part.
 *  @param {string} a @param {string} b @returns {boolean} */
export function older(a, b) {
  const x = a.split(".").map(Number);
  const y = b.split(".").map(Number);

  for (let i = 0; i < Math.max(x.length, y.length); i++) {
    const p = x[i] ?? 0;
    const q = y[i] ?? 0;

    if (p !== q) {
      return p < q;
    }
  }

  return false;
}

/** newest is the release to install: this mod's, or a newer one the service
 *  found. @param {Copy | null} copy @returns {string} */
export function newest(copy) {
  return copy?.latest && older(VERSION, copy.latest) ? copy.latest : VERSION;
}

/** install puts a release in place from a new tab, and waits for
 *  the status it writes. @param {string} version @param {Step} step @param {Sleep} sleep */
export async function install(version, step, sleep) {
  if (!/^[0-9]{1,6}\.[0-9]{1,6}\.[0-9]{1,6}$/.test(version)) {
    throw new Error("The release version is invalid.");
  }

  const folder = `data/auto-title-install-${randomUUID()}`;
  const dir = `${appHome()}/${folder}`;

  await mkdir(dir, { recursive: true });

  try {
    step(`Installing ${version} in a new tab…`);

    // CMD receives only a version and a generated relative path, never a user path.
    const argv = os === "windows"
      ? ["cmd.exe", "/d", "/c", "install.cmd", version, `../../${folder}`]
      : ["sh", `${MOD}/install.sh`, version, dir];

    run("terminal.open", { argv, folder: MOD, name: "Auto Title install" });

    const [code, reason] = await status(dir, sleep);

    if (code !== "0") {
      throw new Error(reason ?? "The install failed.");
    }
  } finally {
    for (const file of await readdir(dir)) {
      await unlink(`${dir}/${file}`);
    }

    await rmdir(dir);
  }
}

/** status is what the installer wrote to dir, once it has.
 *  @param {string} dir @param {Sleep} sleep @returns {Promise<string[]>} */
async function status(dir, sleep) {
  for (let waited = 0; waited < INSTALL_TIMEOUT_MS; waited += POLL_MS) {
    if (await exists(`${dir}/status`)) {
      const text = (await readFile(`${dir}/status`, "utf8")).trim();

      if (text) {
        return text.split(/\r?\n/);
      }
    }

    await sleep(POLL_MS);
  }

  throw new Error("The install tab did not finish.");
}

/** start has the copy install itself again, which starts its service with no
 *  download: the copy keeps its own version. */
export async function start() {
  const path = await copy();

  if (!path) {
    throw new Error("The installed copy is missing.");
  }

  await output(path, ["desktop", "install"], "desktop install");
}
