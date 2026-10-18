// Shared state of the Settings screen: the settings as last loaded, the
// edited copy (draft), and whether the worker is down (then the app saves
// the file itself, through the same Go code the worker uses).
import { reactive } from "vue";
import { Worker, System, type SettingsInfo, type SettingsPatch } from "../../api";
import { store, toast, fail } from "../../store";

// The fields edited in place and saved with the Save button. Workspace
// changes (tiers, rules, adding, removing) are saved at once instead.
export const draftKeys = [
  "pollTimeoutSeconds", "claudeBinary", "settingSources", "strictMcpConfig", "gitBashPath",
  "maxConcurrentJobs", "jobTimeoutMinutes", "maxTurns", "maxBudgetUsd", "maxQueuePerSession",
  "maxOutputBytes", "maxChildRssMb", "bgWaitCeilingMs", "childPath", "extraDisallowedTools",
  "automationEnabled", "screenControl", "camera", "openUrls", "browserApp", "shutdownCommand",
  "stateDir", "logDir", "logLevel", "jobsDays", "jobEventsDays", "auditDays", "processedUpdatesDays",
] as const;
export type DraftKey = (typeof draftKeys)[number];
// Lists are never null in the draft (Go sends empty lists as null).
export type Draft = { [K in DraftKey]: NonNullable<SettingsInfo[K]> };

export const st = reactive({
  s: null as SettingsInfo | null,
  draft: {} as Draft,
  offline: false,
  loading: false,
  saving: false,
});

const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);
const copy = <T>(v: T): T => (v === undefined ? v : JSON.parse(JSON.stringify(v)));
// listKeys default to [] (Go sends an empty list as null; an older worker may not send a key).
const listKeys: readonly string[] = ["settingSources", "childPath", "extraDisallowedTools"];
const value = (x: object | null, k: string) => (x as Record<string, unknown> | null)?.[k] ?? (listKeys.includes(k) ? [] : undefined);

export function fill(x: SettingsInfo) {
  st.s = x;
  const d: Record<string, unknown> = {};
  for (const k of draftKeys) d[k] = copy(value(x, k));
  st.draft = d as Draft;
}

/** changes is the patch for the edited fields. */
export function changes(): SettingsPatch {
  const p: Record<string, unknown> = {};
  if (!st.s) return p;
  for (const k of draftKeys) if (!same(value(st.s, k), st.draft[k])) p[k] = copy(st.draft[k]);
  return p as SettingsPatch;
}

export const dirty = () => Object.keys(changes()).length > 0;

export async function load() {
  st.loading = true;
  try {
    st.offline = !store.connection.connected;
    fill(st.offline ? await System.SettingsOffline() : await Worker.Settings());
  } catch (e) {
    st.s = null;
    fail(e);
  } finally {
    st.loading = false;
  }
}

async function send(p: SettingsPatch) {
  return st.offline ? System.SetSettingsOffline(p) : Worker.SetSettings(p);
}

const savedMsg = () => (st.offline ? "Saved. It applies when the worker starts." : "Saved. Restart the worker to apply.");

/** save sends the edited fields. */
export async function save() {
  st.saving = true;
  try {
    fill(await send(changes()));
    toast(savedMsg(), "ok");
  } catch (e) {
    fail(e);
  } finally {
    st.saving = false;
  }
}

/** patchNow saves one change at once (workspaces), keeping other edits. */
export async function patchNow(p: SettingsPatch, msg = savedMsg()) {
  const keep = changes();
  try {
    fill(await send(p));
    Object.assign(st.draft, keep);
    toast(msg, "ok");
    return true;
  } catch (e) {
    fail(e);
    return false;
  }
}

/** resetSection restores a section's defaults (control.Section*). */
export async function resetSection(section: string) {
  try {
    fill(st.offline ? await System.ResetSettingsOffline(section) : await Worker.ResetSettings(section));
    toast(section === "all" ? "All settings reset. " + savedMsg() : "Section reset. " + savedMsg(), "ok");
  } catch (e) {
    fail(e);
  }
}
