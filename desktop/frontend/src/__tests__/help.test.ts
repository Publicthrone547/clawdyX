// Help: the topic list, search, and the only two kinds of links it follows.
import { describe, expect, it, vi, beforeEach } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { fixtures, platform } from "./mocks";

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));
const calls: string[] = [];
vi.mock("../api", () => ({
  System: {
    HelpTopics: () => Promise.resolve(fixtures.HelpTopics),
    Help: (id: string) => {
      calls.push("help " + id);
      return Promise.resolve({ ...fixtures.Help, id });
    },
    HasLink: (key: string) => Promise.resolve(key === "releases"),
    Open: (key: string) => {
      calls.push("open " + key);
      return Promise.resolve();
    },
  },
  errMsg: (e: unknown) => String(e),
}));

import { store } from "../store";
import Help from "../views/Help.vue";

beforeEach(() => {
  calls.length = 0;
  store.helpTopic = "";
  store.platform = platform("windows") as any;
});

describe("Help", () => {
  it("shows the first topic and follows help: and open: links only", async () => {
    const w = mount(Help, { attachTo: document.body });
    await flushPromises();
    expect(calls).toEqual(["help getting-started"]);
    expect(w.text()).toContain("this PC");
    expect(w.find("article").text()).toContain("background worker");

    await w.get('a[href="help:terminal"]').trigger("click");
    await flushPromises();
    expect(calls).toContain("help terminal");
    expect(store.helpTopic).toBe("terminal");

    await w.get('a[href="open:releases"]').trigger("click");
    await flushPromises();
    expect(calls).toContain("open releases");

    const n = calls.length;
    const a = document.createElement("a");
    a.setAttribute("href", "https://example.com");
    a.textContent = "elsewhere";
    w.get("article").element.appendChild(a);
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    a.dispatchEvent(ev);
    await flushPromises();
    expect(ev.defaultPrevented).toBe(true);
    expect(calls.length).toBe(n);
    w.unmount();
  });

  it("filters topics by title", async () => {
    const w = mount(Help);
    await flushPromises();
    await w.get('input[type="search"]').setValue("term");
    const items = w.findAll('ul[aria-label="Help topics"] button').map((b) => b.text());
    expect(items).toEqual(["Terminal setup"]);
    await w.get('input[type="search"]').setValue("zzz");
    expect(w.text()).toContain("No topic matches.");
    w.unmount();
  });

  it("reopens the topic shown last", async () => {
    store.helpTopic = "terminal";
    const w = mount(Help);
    await flushPromises();
    expect(calls).toEqual(["help terminal"]);
    w.unmount();
  });
});
