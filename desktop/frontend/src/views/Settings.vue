<script setup lang="ts">
import { reactive, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import { Worker, System, Setup, errMsg, type SettingsInfo, type ModelsInfo, type D, type SettingsPatch } from "../api";
import { store, toast, fail, refreshService } from "../store";
import { theme, setTheme, type Theme } from "../prefs";

const s = ref<SettingsInfo | null>(null);
const models = ref<ModelsInfo | null>(null);
const app = ref<D.AppInfo | null>(null);
const f = reactive({ maxConcurrentJobs: 2, jobTimeoutMinutes: 15, maxTurns: 60, automationEnabled: false, screenControl: false, openUrls: true, browserApp: "", shutdownCommand: false, logLevel: "info", jobsDays: 90, jobEventsDays: 14, auditDays: 180 });
const th = ref<Theme>(theme());
const update = ref<D.UpdateInfo | null>(null);
const checking = ref(false);
async function checkUpdate() {
  checking.value = true;
  try {
    update.value = await System.CheckForUpdate();
    if (!update.value.newer) toast(`clawdyX ${update.value.current} is the latest release.`, "ok");
  } catch (e) {
    fail(e);
  } finally {
    checking.value = false;
  }
}
// The worker reports its own version; after an app update it runs the old one until restarted.
const workerVersion = () => store.status?.version ?? "";
const workerOutdated = () => !!app.value?.version && !!workerVersion() && workerVersion() !== app.value.version && !!store.service?.usesAppWorker;
const saving = ref(false);

function fill(x: SettingsInfo) {
  s.value = x;
  Object.assign(f, { maxConcurrentJobs: x.maxConcurrentJobs, jobTimeoutMinutes: x.jobTimeoutMinutes, maxTurns: x.maxTurns, automationEnabled: x.automationEnabled, screenControl: x.screenControl, openUrls: x.openUrls, browserApp: x.browserApp, shutdownCommand: x.shutdownCommand, logLevel: x.logLevel, jobsDays: x.jobsDays, jobEventsDays: x.jobEventsDays, auditDays: x.auditDays });
}
async function load() {
  app.value = await System.Info().catch(() => null);
  refreshService();
  if (!store.connection.connected) return;
  try {
    fill(await Worker.Settings());
    models.value = await Worker.Models();
  } catch (e) {
    fail(e);
  }
}
watch(() => store.connection.connected, load, { immediate: true });

function changed(): SettingsPatch {
  const p: Record<string, unknown> = {};
  if (!s.value) return p;
  for (const k of Object.keys(f) as (keyof typeof f)[]) if ((s.value as any)[k] !== f[k]) p[k] = f[k];
  return p as SettingsPatch;
}
const riskAsk = ref(false);
function save() {
  const p = changed();
  if ((p.automationEnabled === true && !s.value?.automationEnabled) || (p.screenControl === true && !s.value?.screenControl)) riskAsk.value = true;
  else doSave(p);
}
async function doSave(p: SettingsPatch = changed()) {
  riskAsk.value = false;
  saving.value = true;
  try {
    fill(await Worker.SetSettings(p));
    toast("Saved to config.json. Restart the worker to apply.", "ok");
  } catch (e) {
    fail(e);
  } finally {
    saving.value = false;
  }
}
async function patch(p: SettingsPatch, msg: string) {
  try {
    fill(await Worker.SetSettings(p));
    toast(msg, "ok");
  } catch (e) {
    fail(e);
  }
}
async function addWorkspace() {
  try {
    const fc = await Setup.ChooseFolder();
    if (!fc?.path) return;
    if (!fc.ok) return toast(fc.error ?? "That folder cannot be used", "danger");
    if (fc.warning) toast(fc.warning);
    await patch({ addWorkspace: { name: "", path: fc.path, defaultMode: "observe", allowedCommands: [], deniedCommands: [], isDefault: false } }, "Workspace added. Restart the worker to use it.");
  } catch (e) {
    fail(e);
  }
}
async function setDefaultWs(name: string) {
  try {
    fill(await Worker.SetDefaultWorkspace(name));
    toast(`New sessions will use ${name}.`, "ok");
  } catch (e) {
    fail(e);
  }
}
async function restartWorker() {
  try {
    await Worker.RestartWorker();
    toast("Restarting the worker…", "ok");
  } catch (e) {
    fail(e);
  }
}
async function defaults(kind: "model" | "effort", v: string) {
  try {
    const r = kind === "model" ? await Worker.SetModel("", v, true) : await Worker.SetEffort("", v, true);
    toast(r.message, "ok");
    models.value = await Worker.Models();
  } catch (e) {
    fail(e);
  }
}
const defaultModel = ref("");
async function launchAtLogin(on: boolean) {
  try {
    const v = await System.SetLaunchAtLogin(on);
    if (app.value) app.value.launchAtLogin = v;
  } catch (e) {
    toast(errMsg(e), "danger");
  }
}
const svcBusy = ref(false);
async function svc(fn: () => Promise<string>) {
  svcBusy.value = true;
  try {
    toast((await fn()) || "Done", "ok");
  } catch (e) {
    fail(e);
  } finally {
    svcBusy.value = false;
    refreshService();
  }
}
const uninstallAsk = ref(false);
const purgeState = ref(false);
function doUninstall() {
  uninstallAsk.value = false;
  const p = purgeState.value;
  svc(() => System.UninstallService(p));
}
</script>

<template>
  <Page title="Settings" subtitle="Changes to the worker's settings are saved in config.json and apply after a restart.">
    <template #actions>
      <button class="btn btn-primary" :disabled="saving || !Object.keys(changed()).length" @click="save">Save</button>
    </template>
    <div v-if="s?.restartPending" class="card mb-4 flex items-center gap-4 px-5 py-3" style="border-color: var(--warn)">
      <p class="flex-1">Saved changes take effect after the worker restarts. Running jobs are interrupted, not re-run.</p>
      <button class="btn" @click="restartWorker">Restart worker</button>
    </div>

    <section class="card px-5 py-4" aria-labelledby="about-h">
      <h2 id="about-h" class="font-semibold">About clawdyX</h2>
      <dl class="mt-3 grid grid-cols-[10rem_1fr] gap-y-1.5">
        <dt class="text-muted">App version</dt><dd class="mono">{{ app?.version || "—" }}</dd>
        <dt class="text-muted">Worker version</dt>
        <dd class="flex flex-wrap items-center gap-2">
          <span class="mono">{{ workerVersion() || "—" }}</span>
          <Badge v-if="workerOutdated()" tone="warn" label="older than the app: restart the worker" />
        </dd>
        <dt class="text-muted">Claude Code</dt><dd class="mono">{{ store.status?.claude.version || "—" }}</dd>
      </dl>
      <div v-if="update?.newer" class="mt-3 rounded-md border border-line bg-surface-2 px-3 py-2">
        <b>clawdyX {{ update.latest }} is available.</b> Download the .dmg from the release page, quit clawdyX, replace it in Applications, then restart the worker.
      </div>
      <div class="mt-3 flex flex-wrap gap-2">
        <button v-if="update?.newer" class="btn btn-primary" @click="System.OpenRelease(update.latest).catch(fail)">Get {{ update.latest }}</button>
        <button class="btn" :disabled="checking" @click="checkUpdate">{{ checking ? "Checking…" : "Check for updates" }}</button>
        <button v-if="app?.version" class="btn" @click="System.OpenRelease('').catch(fail)">What's new in {{ app.version }}</button>
        <button class="btn" @click="System.Open('releases').catch(fail)">All releases</button>
        <button v-if="workerOutdated()" class="btn" :disabled="svcBusy" @click="svc(System.RestartService)">Restart worker</button>
      </div>
      <p class="mt-2 text-xs text-muted">Checking for updates asks GitHub for the latest release, only when you click. clawdyX never checks on its own.</p>
    </section>

    <section class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">General</h2>
      <div class="mt-3 grid grid-cols-2 gap-4">
        <div>
          <label class="label" for="theme">Appearance</label>
          <select id="theme" v-model="th" class="input mt-1" @change="setTheme(th)"><option value="system">Follow macOS</option><option value="light">Light</option><option value="dark">Dark</option></select>
        </div>
        <div>
          <span class="label">Open clawdyX at login</span>
          <label class="mt-2 flex items-center gap-2"><input type="checkbox" :checked="app?.launchAtLogin" @change="launchAtLogin(($event.target as HTMLInputElement).checked)" /> Open the app (menu-bar item) when you log in</label>
          <p class="mt-1 text-xs text-muted">The worker starts at login on its own; this is only the window and menu-bar item.</p>
        </div>
      </div>
    </section>

    <section v-if="s" class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">Claude defaults for new sessions</h2>
      <div class="mt-3 grid grid-cols-3 gap-4">
        <div>
          <label class="label" for="dmodel">Model · now {{ models?.defaultModel }}</label>
          <div class="mt-1 flex gap-2">
            <input id="dmodel" v-model="defaultModel" class="input" list="dmodels" placeholder="fable, opus, sonnet…" />
            <datalist id="dmodels"><option v-for="m in models?.builtin ?? []" :key="m" :value="m" /></datalist>
            <button class="btn" :disabled="!defaultModel" @click="defaults('model', defaultModel)">Set</button>
          </div>
        </div>
        <div>
          <label class="label" for="deffort">Effort · now {{ models?.defaultEffort }}</label>
          <select id="deffort" class="input mt-1" :value="models?.defaultEffort" @change="defaults('effort', ($event.target as HTMLSelectElement).value)">
            <option v-for="e in models?.effortLevels ?? []" :key="e" :value="e">{{ e }}</option>
          </select>
        </div>
        <div>
          <label class="label" for="dws">Workspace</label>
          <select id="dws" class="input mt-1" :value="s.defaultWorkspace" @change="setDefaultWs(($event.target as HTMLSelectElement).value)">
            <option v-for="w in s.workspaces" :key="w.name" :value="w.name">{{ w.name }}</option>
          </select>
        </div>
      </div>
    </section>

    <section v-if="s" class="card mt-4">
      <div class="flex items-center justify-between px-5 pt-4">
        <div>
          <h2 class="font-semibold">Workspaces</h2>
          <p class="text-xs text-muted">Claude can only work inside these folders. The tier is the one new sessions start in.</p>
        </div>
        <button class="btn" @click="addWorkspace">Add folder…</button>
      </div>
      <table class="tbl mt-3">
        <thead><tr><th>Name</th><th>Folder</th><th>Starting tier</th><th>Allowed commands (assisted)</th><th></th></tr></thead>
        <tbody>
          <tr v-for="w in s.workspaces" :key="w.name">
            <td><b>{{ w.name }}</b> <Badge v-if="w.isDefault" tone="accent" label="default" /></td>
            <td class="mono">{{ w.path }}</td>
            <td>
              <select class="input py-1" :value="w.defaultMode" :aria-label="`Starting tier of ${w.name}`" @change="patch({ workspaceModes: { [w.name]: ($event.target as HTMLSelectElement).value } }, 'Saved. Restart the worker to apply.')">
                <option value="observe">observe</option><option value="assisted">assisted</option>
              </select>
            </td>
            <td class="max-w-64 truncate text-xs text-muted mono" :title="(w.allowedCommands ?? []).join(', ')">{{ (w.allowedCommands ?? []).join(", ") }}</td>
            <td class="text-right"><button class="btn btn-danger btn-sm" :disabled="(s.workspaces?.length ?? 0) < 2" @click="patch({ removeWorkspace: w.name }, 'Workspace removed. Restart the worker to apply.')">Remove</button></td>
          </tr>
        </tbody>
      </table>
    </section>

    <section v-if="s" class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">Limits</h2>
      <div class="mt-3 grid grid-cols-3 gap-4">
        <div><label class="label" for="conc">Jobs at once (1–4)</label><input id="conc" v-model.number="f.maxConcurrentJobs" type="number" min="1" max="4" class="input mt-1" /></div>
        <div><label class="label" for="tmo">Job timeout (minutes)</label><input id="tmo" v-model.number="f.jobTimeoutMinutes" type="number" min="1" max="120" class="input mt-1" /></div>
        <div><label class="label" for="turns">Max turns per job</label><input id="turns" v-model.number="f.maxTurns" type="number" min="1" max="500" class="input mt-1" /></div>
      </div>
    </section>

    <section v-if="s" class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">Features</h2>
      <div class="mt-3 space-y-2">
        <label class="flex items-start gap-2"><input v-model="f.automationEnabled" type="checkbox" class="mt-0.5" /><span><b>Automation tier available</b><span class="block text-xs text-muted">Lets a session switch to automation: Claude runs commands that Claude Code's safety classifier approves, anywhere on this Mac. Each session still needs an explicit switch.</span></span></label>
        <label class="flex items-start gap-2"><input v-model="f.screenControl" type="checkbox" class="mt-0.5" /><span><b>Screen features</b><span class="block text-xs text-muted">Monitors, /screen and /look, and Claude's screenshot, mouse and keyboard tools in automation sessions. Needs macOS permissions.</span></span></label>
        <label class="flex items-start gap-2"><input v-model="f.openUrls" type="checkbox" class="mt-0.5" /><span><b>/open in Telegram</b><span class="block text-xs text-muted">Owner-typed command that opens a website in</span></span></label>
        <div class="ml-6 w-64"><input v-model="f.browserApp" class="input" aria-label="Browser app" /></div>
        <label class="flex items-start gap-2"><input v-model="f.shutdownCommand" type="checkbox" class="mt-0.5" /><span><b>/shutdown in Telegram</b><span class="block text-xs text-muted">Lets the owner stop the worker from Telegram. Starting it again then needs this Mac.</span></span></label>
      </div>
    </section>

    <section v-if="s" class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">Logs and retention</h2>
      <div class="mt-3 grid grid-cols-4 gap-4">
        <div><label class="label" for="lvl">Log level</label><select id="lvl" v-model="f.logLevel" class="input mt-1"><option>debug</option><option>info</option><option>warn</option><option>error</option></select></div>
        <div><label class="label" for="jd">Keep jobs (days)</label><input id="jd" v-model.number="f.jobsDays" type="number" min="1" class="input mt-1" /></div>
        <div><label class="label" for="ed">Keep job activity (days)</label><input id="ed" v-model.number="f.jobEventsDays" type="number" min="1" class="input mt-1" /></div>
        <div><label class="label" for="ad">Keep audit log (days)</label><input id="ad" v-model.number="f.auditDays" type="number" min="1" class="input mt-1" /></div>
      </div>
      <p class="mt-2 text-xs text-muted">Logs rotate at 20 MB × 5 in <span class="mono">{{ app?.logDir }}</span>. <a href="#" class="underline" @click.prevent="System.RevealLogs()">Show in Finder</a></p>
    </section>

    <section class="card mt-4 px-5 py-4">
      <h2 class="font-semibold">Background worker</h2>
      <dl class="mt-3 grid grid-cols-[10rem_1fr] gap-y-1.5">
        <dt class="text-muted">Service</dt><dd><Badge :tone="store.service?.pid ? 'ok' : store.service?.installed ? 'warn' : 'muted'" :label="store.service?.pid ? `running (pid ${store.service.pid})` : store.service?.installed ? 'installed, not running' : 'not installed'" /></dd>
        <dt class="text-muted">Program</dt><dd class="mono">{{ store.service?.binary || "—" }}</dd>
        <dt class="text-muted">Config</dt><dd class="mono">{{ app?.configPath }}</dd>
      </dl>
      <div class="mt-3 flex flex-wrap gap-2">
        <button v-if="store.service?.inBundle && !store.service?.usesAppWorker" class="btn" :disabled="svcBusy" @click="svc(System.InstallService)">Use the app's worker</button>
        <button class="btn" :disabled="svcBusy" @click="svc(System.RestartService)">Restart</button>
        <button class="btn" :disabled="svcBusy || !store.service?.loaded" @click="svc(System.StopService)">Stop</button>
        <button class="btn" :disabled="svcBusy || !!store.service?.pid" @click="svc(store.service?.installed ? System.StartService : System.InstallService)">Start</button>
        <button class="btn btn-danger" :disabled="svcBusy || !store.service?.installed" @click="uninstallAsk = true">Uninstall…</button>
      </div>
    </section>

    <Modal v-if="riskAsk" title="Turn on a powerful feature?" @close="riskAsk = false">
      <p v-if="changed().automationEnabled">Automation lets Claude run shell commands anywhere on this Mac when Claude Code's classifier approves them. Deny rules and secret paths stay blocked, and each session still needs an explicit switch.</p>
      <p v-if="changed().screenControl" class="mt-2">Screen features let you (and, in automation sessions, Claude) capture your displays. macOS will ask for Screen Recording and Accessibility permission.</p>
      <template #actions>
        <button class="btn" @click="riskAsk = false">Cancel</button>
        <button class="btn btn-primary" @click="doSave()">Turn on and save</button>
      </template>
    </Modal>
    <Modal v-if="uninstallAsk" title="Uninstall the background worker?" @close="uninstallAsk = false">
      <p>The worker stops and no longer starts at login. Claude Code, its login and your project folders are never touched.</p>
      <label class="mt-3 flex items-center gap-2"><input v-model="purgeState" type="checkbox" /> Also delete clawdyX's state (sessions, jobs, audit log) and logs</label>
      <p class="mt-2 text-xs text-muted">Settings (<span class="mono">{{ app?.configPath }}</span>) and the Keychain token are kept so you can reinstall; remove them by hand if you want.</p>
      <template #actions>
        <button class="btn" @click="uninstallAsk = false">Cancel</button>
        <button class="btn btn-danger" @click="doUninstall">Uninstall</button>
      </template>
    </Modal>
  </Page>
</template>
