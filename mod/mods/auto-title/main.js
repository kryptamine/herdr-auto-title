// Auto Title for the desktop app: a popup that says whether the desktop mode
// runs at login, with the buttons that install, start or update it.
// What it does to the machine is in ./service.js; this is only the popup.

import { View, div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme } from "herdr";
import * as service from "./service.js";

/** @typedef {import("gpui-kit").AsyncContext} AsyncContext */
/** @typedef {import("./service.js").Copy} Copy */
/** @typedef {import("./service.js").Step} Step */
/** @typedef {import("./service.js").Sleep} Sleep */
/** @typedef {"dim" | "working" | "blocked" | "done"} Tone */
/** @typedef {Record<Tone | "popover" | "border" | "foreground" | "muted" | "solid" | "solid_foreground", `#${string}`>} Theme
 *  the colors this popup uses; herdr.d.ts types theme() as plain strings */
/** @typedef {{ id: string, label: string, work: (step: Step, sleep: Sleep) => Promise<void> }} Action */
/** @typedef {{ tone: Tone, text: string, actions: Action[] }} Shown */
/** @typedef {{ kind: "looking" } | { kind: "absent" } | { kind: "present", copy: Copy } | { kind: "broken", reason: string }} Machine */

/** installing is a button that installs version. @param {string} id @param {string} label
 *  @param {string} version @returns {Action} */
const installing = (id, label, version) => ({
  id,
  label: `${label} ${version}`,
  work: (step, sleep) => service.install(version, step, sleep),
});

const INSTALL = installing("install", "Install", service.VERSION);
const REINSTALL = installing("reinstall", "Reinstall", service.VERSION);

/** @type {Action} */
const START = { id: "start", label: "Start", work: () => service.start() };

// The gaps are mod.json's overlay size less the card's 440×150.
const RIGHT_GAP = 5;
const BOTTOM_GAP = 34;

/** @param {unknown} e @returns {string} */
const message = (e) => (e instanceof Error ? e.message : String(e));

export default class AutoTitle extends View {
  /** @type {Machine} */
  machine = { kind: "looking" };

  // What a button is doing now, and why the last one failed.
  busy = "";
  error = "";

  /** @param {unknown} _props @param {AsyncContext} cx */
  init(_props, cx) {
    if (service.supported) {
      this.refresh(cx);
    }
  }

  /** @param {AsyncContext} cx */
  refresh(cx) {
    cx.spawn(async (cx) => {
      try {
        const copy = await service.installed();

        this.machine = copy ? { kind: "present", copy } : { kind: "absent" };
      } catch (e) {
        this.machine = { kind: "broken", reason: message(e) };
      }

      cx.notify();
    });
  }

  /** act runs one button's work, showing each step it reports, then reads the
   *  machine again whatever happened. @param {AsyncContext} cx @param {Action["work"]} work */
  act(cx, work) {
    this.busy = "Starting…";
    this.error = "";
    cx.notify();

    cx.spawn(async (cx) => {
      /** @type {Step} */
      const step = (text) => {
        this.busy = text;
        cx.notify();
      };

      try {
        await work(step, (ms) => cx.sleep(ms));
      } catch (e) {
        this.error = message(e);
      }

      this.busy = "";
      this.refresh(cx);
    });
  }

  /** shown is the popup for the machine as last read: the status and the
   *  buttons come from one place, so they cannot disagree. @returns {Shown} */
  shown() {
    if (!service.supported) {
      return { tone: "dim", text: service.unsupported, actions: [] };
    }

    if (this.busy) {
      return { tone: "working", text: this.busy, actions: [] };
    }

    const state = machineShown(this.machine);

    // A failed button says why over the state and keeps its buttons.
    return this.error ? { tone: "blocked", text: this.error, actions: state.actions } : state;
  }

  render() {
    const t = /** @type {Theme} */ (theme());
    const { tone, text, actions } = this.shown();

    // The overlay has no frame and is larger than the card, which moves the
    // card in from the window's corner; the app gives only 8px.
    return v_flex()
      .id("root")
      .size_full()
      .pr(RIGHT_GAP)
      .pb(BOTTOM_GAP)
      .child(this.card(t, tone, text, actions));
  }

  /** @param {Theme} t @param {Tone} tone @param {string} text @param {Action[]} actions */
  card(t, tone, text, actions) {
    return v_flex()
      .id("card")
      .flex_1()
      .min_h(0)
      .p(18)
      .gap(12)
      .rounded(10)
      .border_1()
      .border_color(t.border)
      .bg(t.popover)
      .shadow_lg()
      .text_size(13)
      .child(
        h_flex()
          .gap(8)
          .items_center()
          .child(div().size(8).rounded(4).flex_none().bg(t[tone]))
          .child(
            div().flex_1().text_size(15).font_semibold().text_color(t.foreground).child("Auto Title"),
          )
          .child(div().text_size(11).text_color(t.dim).child("Esc")),
      )
      .child(
        div()
          .flex_1()
          .min_h(0)
          .text_color(tone === "blocked" ? t.blocked : t.muted)
          .child(text),
      )
      .when(actions.length > 0, (el) =>
        el.child(
          h_flex()
            .justify_end()
            .gap(8)
            .text_size(12)
            .children(
              actions.map((a) =>
                button(t, a, (cx) => {
                  this.act(cx, a.work);
                }),
              ),
            ),
        ),
      );
  }
}

/** machineShown is the status and buttons each state of the machine gets.
 *  @param {Machine} machine @returns {Shown} */
function machineShown(machine) {
  switch (machine.kind) {
    case "looking":
      return { tone: "dim", text: "Checking…", actions: [] };
    case "absent":
      return {
        tone: "dim",
        text: "Not installed. Tabs keep the names Herdr gives them.",
        actions: [INSTALL],
      };
    case "broken":
      return { tone: "blocked", text: machine.reason, actions: [REINSTALL] };
    case "present":
      return copyShown(machine.copy);
  }
}

/** copyShown is an installed copy's status and buttons. A copy newer than the
 *  newest release known is never downgraded. @param {Copy} copy @returns {Shown} */
function copyShown(copy) {
  const { version, running } = copy;
  const target = service.newest(copy);
  const outdated = service.older(version, target);
  const update = installing("update", "Update to", target);
  const news = outdated ? [`Version ${target} is out.`] : [];

  if (!running) {
    return {
      tone: "blocked",
      text: [`Version ${version} is installed but not running.`, ...news].join(" "),
      // An update installs the copy, which starts it too.
      actions: [outdated ? update : START],
    };
  }

  return {
    tone: "done",
    text: outdated
      ? [`Version ${version} is running.`, ...news].join(" ")
      : `Version ${version} is running and names your tabs.`,
    actions: outdated ? [update] : [],
  };
}

/** @param {Theme} t @param {Action} action @param {(cx: AsyncContext) => void} onClick */
function button(t, { id, label }, onClick) {
  return h_flex()
    .id(id)
    .h(32)
    .px(14)
    .flex_none()
    .items_center()
    .rounded(8)
    .cursor_pointer()
    .bg(t.solid)
    .text_color(t.solid_foreground)
    .hover((el) => el.opacity(0.88))
    .on_click((_e, cx) => {
      onClick(cx);
    })
    .child(label);
}
