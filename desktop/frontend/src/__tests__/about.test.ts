// About: the rows of this computer and the download of an update.
import { describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";
import { fixtures, platform } from "./mocks";

vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));
const opened: string[] = [];
vi.mock("../api", () => ({
  System: {
    Info: () => Promise.resolve(fixtures.Info),
    CheckForUpdate: () =>
      Promise.resolve({ current: "1.0.0", latest: "1.0.1", newer: true, url: "https://github.com/Publicthrone547/clawdyX/releases/tag/v1.0.1",
        assetName: "clawdyX-1.0.1-windows-amd64-setup.exe", assetUrl: "https://github.com/Publicthrone547/clawdyX/releases/download/v1.0.1/clawdyX-1.0.1-windows-amd64-setup.exe", assetBytes: 31 * 1024 * 1024, kind: "setup" }),
    OpenDownload: (u: string) => {
      opened.push(u);
      return Promise.resolve();
    },
    RevealFolder: (w: string) => {
      opened.push("folder " + w);
      return Promise.resolve();
    },
  },
  errMsg: (e: unknown) => String(e),
}));

import { store } from "../store";
import About from "../views/settings/About.vue";

describe("About", () => {
  it("shows this computer and offers the download for it", async () => {
    store.platform = { ...platform("windows"), pretty: "Windows 11 23H2" } as any;
    store.service = fixtures.Service;
    const w = mount(About);
    await flushPromises();
    const text = w.text();
    expect(text).toContain("Windows 11 23H2 · amd64");
    expect(text).toContain("login task: running (pid 42)");
    expect(text).toContain("Show in File Explorer");
    const show = w.findAll("button").find((b) => b.text() === "Show in File Explorer");
    await show!.trigger("click");
    expect(opened).toContain("folder config");

    await w.findAll("button").find((b) => b.text() === "Check for updates")!.trigger("click");
    await flushPromises();
    const dl = w.findAll("button").find((b) => b.text().startsWith("Download clawdyX-1.0.1-windows-amd64-setup.exe"));
    expect(dl, w.text()).toBeTruthy();
    expect(dl!.text()).toContain("31");
    await dl!.trigger("click");
    expect(opened.at(-1)).toContain("/releases/download/v1.0.1/");
    w.unmount();
  });

  it("names the Linux session", async () => {
    store.platform = { ...platform("linux"), pretty: "Ubuntu 24.04 LTS", session: "wayland" } as any;
    const w = mount(About);
    await flushPromises();
    expect(w.text()).toContain("Ubuntu 24.04 LTS · amd64 · Wayland (GNOME)");
    w.unmount();
  });
});
