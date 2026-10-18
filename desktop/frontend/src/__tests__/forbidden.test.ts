// The wording guard: every view, rendered with the words of each OS, must
// not name another OS or an OS-only product (the lists in forbidden.json,
// shared with internal/platform). Views are mounted directly with mocked
// bindings and fixture data.
import { describe, expect, it, vi } from "vitest";
import { flushPromises, mount, type VueWrapper } from "@vue/test-utils";
import { forbiddenWords, oses } from "./forbidden";
import { api, fixtures, platform } from "./mocks";

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));
vi.mock("../api", () => api);

import { store } from "../store";
import Overview from "../views/Overview.vue";
import Telegram from "../views/Telegram.vue";
import Sessions from "../views/Sessions.vue";
import Jobs from "../views/Jobs.vue";
import Monitors from "../views/Monitors.vue";
import Settings from "../views/Settings.vue";
import Diagnostics from "../views/Diagnostics.vue";
import Wizard from "../views/Wizard.vue";
import Help from "../views/Help.vue";
import App from "../App.vue";
import Rules from "../views/settings/Rules.vue";

const views: Record<string, unknown> = { Overview, Telegram, Sessions, Jobs, Monitors, Settings, Diagnostics, Wizard, Help };

/** visible collects the text and the labels people see or hear. */
function visible(w: VueWrapper): string {
  const parts = [w.text()];
  for (const el of w.element.querySelectorAll?.("*") ?? []) {
    for (const a of ["title", "aria-label", "placeholder", "alt"]) {
      const v = el.getAttribute(a);
      if (v) parts.push(v);
    }
  }
  return parts.join("\n");
}

function setPlatform(os: (typeof oses)[number]) {
  const p = platform(os);
  fixtures.Platform = p;
  fixtures.SettingsOffline = fixtures.Settings;
  store.platform = p as any;
  store.connection = fixtures.Connection;
  store.status = fixtures.Status;
  store.service = fixtures.Service;
  store.setup = fixtures.State;
  store.ready = true;
}

describe.each(oses)("views on %s", (os) => {
  it.each(Object.keys(views))("%s names no other OS", async (name) => {
    setPlatform(os);
    const w = mount(views[name] as any, { attachTo: document.body });
    await flushPromises();
    const text = visible(w);
    w.unmount();
    expect(text.length, `${name} rendered nothing`).toBeGreaterThan(80);
    expect(forbiddenWords(os, text), `${name} on ${os}`).toEqual([]);
  });
  it("Settings uses this OS's names", async () => {
    setPlatform(os);
    const w = mount(Settings, { attachTo: document.body });
    await flushPromises();
    const text = visible(w);
    w.unmount();
    const p = platform(os);
    expect(text).toContain("Show in " + p.words.fileManager);
    expect(text).toContain(p.words.launchAtLogin);
  });
  it("the workspace rule editor names no other OS", async () => {
    setPlatform(os);
    const w = mount(Rules, { props: { ws: fixtures.Settings.workspaces[0] }, attachTo: document.body });
    await flushPromises();
    const bad = forbiddenWords(os, visible(w));
    w.unmount();
    expect(bad).toEqual([]);
  });
  it("the sidebar shows this OS's shortcut modifier", async () => {
    setPlatform(os);
    const w = mount(App, { global: { stubs: { Overview: true, Wizard: true, Toasts: true } } });
    await flushPromises();
    const text = visible(w);
    w.unmount();
    expect(forbiddenWords(os, text)).toEqual([]);
    expect(text).toContain(os === "darwin" ? "⌘1" : "Ctrl+1");
  });
});

describe("the matcher", () => {
  it("finds whole words and skips allowed phrases", () => {
    expect(forbiddenWords("windows", "Show in Finder")).toEqual(["Finder"]);
    expect(forbiddenWords("windows", "open a window")).toEqual([]);
    expect(forbiddenWords("darwin", "Ctrl-C stops it")).toEqual([]);
    expect(forbiddenWords("darwin", "run clawdyx.exe")).toEqual([".exe"]);
    expect(forbiddenWords("linux", "drag clawdyX.app")).toEqual([".app"]);
  });
});

describe("shortcuts", () => {
  it.each(oses)("use this OS's modifier on %s", async (os) => {
    setPlatform(os);
    store.view = "overview";
    const w = mount(App, { global: { stubs: { Overview: true, Wizard: true, Toasts: true } } });
    await flushPromises();
    const press = (key: string, mod: { ctrlKey?: boolean; metaKey?: boolean }) => window.dispatchEvent(new KeyboardEvent("keydown", { key, ...mod }));
    const right = os === "darwin" ? { metaKey: true } : { ctrlKey: true };
    const wrong = os === "darwin" ? { ctrlKey: true } : { metaKey: true };
    press("3", wrong);
    expect(store.view).toBe("overview");
    press("3", right);
    expect(store.view).toBe("sessions");
    w.unmount();
  });
});
