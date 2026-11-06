// What this machine has of Auto Title, and how it gets the release this mod
// belongs to. The binary owns the service: it sets it up, takes it away and
// says what state it is in (`desktop status`); this only asks it.

import { platform } from "os";
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

// INSTALL is the script that puts a release in place, run in a tab of its
// own. The path is the mod's folder, which a renamed copy renames too.
const INSTALL = `${appHome()}/mods/auto-title/install.sh`;

// COPIES says where `desktop install` keeps its copy on each OS, as
// internal/autostart does. An OS missing here is not supported yet.
/** @type {Record<string, () => Promise<string>>} */
const COPIES = {
  macos: () => Promise.resolve(`${home()}/Library/Application Support/${NAME}/bin/${NAME}`),
  linux: async () => {
    const data = await output("sh", ["-c", 'printf %s "${XDG_DATA_HOME:-$HOME/.local/share}"']);

    return `${data}/${NAME}/${NAME}`;
  },
};

const os = { darwin: "macos", win32: "windows" }[platform()] ?? platform();
const where = COPIES[os];

export const supported = where !== undefined;

// unsupported is what the popup says on an OS with no copy to ask.
export const unsupported = `Not available on ${{ windows: "Windows" }[os] ?? os} yet.`;

/** copy is the installed copy's path; the strip asks it of no OS it does not support.
 *  @returns {Promise<string>} */
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
  const { code } = await exec("test", ["-x", path]);

  if (code !== 0) {
    return null;
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

/** install has INSTALL put a release in place from a new tab, and waits for
 *  the status it writes. @param {string} version @param {Step} step @param {Sleep} sleep */
export async function install(version, step, sleep) {
  const dir = (await output("mktemp", ["-d"])).trim();

  try {
    step(`Installing ${version} in a new tab…`);
    run("terminal.open", { argv: ["sh", INSTALL, version, dir], name: "Auto Title install" });

    const [code, reason] = await status(dir, sleep);

    if (code !== "0") {
      throw new Error(reason ?? "The install failed.");
    }
  } finally {
    await exec("rm", ["-rf", dir]);
  }
}

/** status is what INSTALL wrote to dir, once it has.
 *  @param {string} dir @param {Sleep} sleep @returns {Promise<string[]>} */
async function status(dir, sleep) {
  for (let waited = 0; waited < INSTALL_TIMEOUT_MS; waited += POLL_MS) {
    const { code, stdout } = await exec("cat", [`${dir}/status`]);

    if (code === 0 && stdout.trim()) {
      return stdout.trim().split("\n");
    }

    await sleep(POLL_MS);
  }

  throw new Error("The install tab did not finish.");
}

/** start has the copy install itself again, which starts its service with no
 *  download: the copy keeps its own version. */
export async function start() {
  await output(await copy(), ["desktop", "install"], "desktop install");
}
