import { reactive } from "vue";
import { Events } from "@wailsio/runtime";
import { Worker, System, Setup, errMsg, type Status, type D } from "./api";

export type View = "overview" | "telegram" | "sessions" | "jobs" | "monitors" | "settings" | "diagnostics";

export interface Toast {
  id: number;
  text: string;
  tone: "ok" | "danger" | "muted";
}

export interface Progress {
  id: number;
  elapsedMs: number;
  tools: number;
  denials: number;
  lastTool: string;
  textTail: string;
}

export interface FinishedJob {
  id: number;
  state: string;
  text: string;
  error: string;
  durationMs: number;
}

export const store = reactive({
  connection: { connected: false, configured: false, error: "" } as D.Connection,
  status: null as Status | null,
  setup: null as D.SetupState | null,
  service: null as D.ServiceInfo | null,
  view: "overview" as View,
  toasts: [] as Toast[],
  capturing: "" as string,
  progress: {} as Record<number, Progress>,
  finished: {} as Record<number, FinishedJob>,
  // Bumped on worker events so views can refetch their lists.
  tick: 0,
  sessionsTick: 0,
  ready: false,
  // Version of this app build ("" until known).
  appVersion: "",
  // The OS and its words (see platform.ts); null until known.
  platform: null as D.PlatformInfo | null,
});

let nextToast = 1;
export function toast(text: string, tone: Toast["tone"] = "muted") {
  const id = nextToast++;
  store.toasts.push({ id, text, tone });
  setTimeout(() => {
    const i = store.toasts.findIndex((t) => t.id === id);
    if (i >= 0) store.toasts.splice(i, 1);
  }, tone === "danger" ? 9000 : 4500);
}

export function fail(e: unknown) {
  toast(errMsg(e), "danger");
}

export async function refreshStatus() {
  if (!store.connection.connected) {
    store.status = null;
    return;
  }
  try {
    store.status = await Worker.Status();
  } catch {
    store.status = null;
  }
}

export async function refreshSetup() {
  try {
    store.setup = await Setup.State();
  } catch (e) {
    fail(e);
  }
}

export async function refreshService() {
  try {
    store.service = await System.Service();
  } catch {
    store.service = null;
  }
}

let statusTimer: ReturnType<typeof setTimeout> | undefined;
function soon() {
  clearTimeout(statusTimer);
  statusTimer = setTimeout(refreshStatus, 250);
}

export function initStore() {
  System.Platform()
    .then((p) => (store.platform = p))
    .catch(() => {});
  System.Info()
    .then((i) => (store.appVersion = i.version))
    .catch(() => {});
  Events.On("worker:connection", (ev) => {
    const c = ev.data as D.Connection;
    store.connection = c;
    refreshStatus();
    refreshService();
    store.tick++;
  });
  Events.On("worker:event", (ev) => {
    const e = ev.data as { type: string; data: any };
    switch (e.type) {
      case "job.progress":
        store.progress[e.data.id] = e.data as Progress;
        return;
      case "job.finished": {
        const j = e.data.job;
        delete store.progress[j.id];
        store.finished[j.id] = { id: j.id, state: e.data.state, text: e.data.text ?? "", error: e.data.error ?? "", durationMs: e.data.durationMs ?? 0 };
        break;
      }
      case "capture.state":
        store.capturing = e.data.capturing ? e.data.display : "";
        return;
      case "sessions.changed":
      case "telegram.command":
        store.sessionsTick++;
        break;
    }
    store.tick++;
    soon();
  });
  Promise.all([Worker.Connection().then((c) => (store.connection = c)), refreshSetup(), refreshService()]).finally(() => {
    store.ready = true;
    refreshStatus();
  });
  setInterval(() => {
    if (document.visibilityState === "visible") refreshStatus();
  }, 5000);
}
