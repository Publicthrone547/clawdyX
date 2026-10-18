<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import Empty from "../components/Empty.vue";
import { Worker, errMsg, type SessionInfo, type HistoryInfo, type ModelsInfo, type SettingsInfo } from "../api";
import { store, toast, fail, refreshStatus } from "../store";
import { words } from "../platform";
import { ago, bytes } from "../util";

const list = ref<SessionInfo[]>([]);
const forgotten = ref<SessionInfo[]>([]);
const showArchived = ref(false);
const showForgotten = ref(false);
const models = ref<ModelsInfo | null>(null);
const settings = ref<SettingsInfo | null>(null);

async function load() {
  if (!store.connection.connected) return;
  try {
    list.value = (await Worker.Sessions(showArchived.value)) ?? [];
    forgotten.value = (await Worker.ForgottenSessions()) ?? [];
    models.value ??= await Worker.Models();
    settings.value ??= await Worker.Settings();
  } catch (e) {
    fail(e);
  }
}
watch(() => [store.sessionsTick, store.connection.connected, showArchived.value], load, { immediate: true });

const active = computed(() => list.value.find((s) => s.active) ?? null);

async function act(p: Promise<{ message: string }>) {
  try {
    toast((await p).message, "ok");
  } catch (e) {
    fail(e);
  } finally {
    load();
    refreshStatus();
  }
}

// dialogs
type Dlg = { kind: "new" | "newf" | "fresh" | "rename" | "forget"; s?: SessionInfo } | null;
const dlg = ref<Dlg>(null);
const name = ref("");
const workspace = ref("");
const deleteHistory = ref(false);
const history = ref<HistoryInfo | null>(null);
const err = ref("");

async function open(kind: NonNullable<Dlg>["kind"], s?: SessionInfo) {
  dlg.value = { kind, s };
  name.value = kind === "rename" ? s?.name ?? "" : "";
  workspace.value = settings.value?.defaultWorkspace ?? "";
  deleteHistory.value = false;
  history.value = null;
  err.value = "";
  const target = kind === "newf" ? active.value : s;
  if ((kind === "newf" || kind === "forget") && target) {
    try {
      history.value = await Worker.SessionHistory(target.label);
    } catch {
      history.value = null;
    }
  }
}
async function confirm() {
  const d = dlg.value!;
  err.value = "";
  try {
    let r: { message: string };
    switch (d.kind) {
      case "new":
        r = await Worker.SessionCreate(name.value.trim(), workspace.value);
        break;
      case "newf":
        r = await Worker.SessionNewForget(name.value.trim(), deleteHistory.value);
        break;
      case "fresh":
        r = await Worker.SessionFresh();
        break;
      case "rename":
        r = await Worker.SessionRename(d.s!.label, name.value.trim());
        break;
      case "forget":
        r = await Worker.SessionForget(d.s!.label, deleteHistory.value);
        break;
    }
    dlg.value = null;
    toast(r.message, "ok");
  } catch (e) {
    err.value = errMsg(e);
  } finally {
    load();
    refreshStatus();
  }
}

// active session settings
const modelInput = ref("");
const busyModel = ref(false);
async function setModel() {
  if (!active.value || !modelInput.value) return;
  busyModel.value = true;
  try {
    toast((await Worker.SetModel(active.value.label, modelInput.value, false)).message, "ok");
    modelInput.value = "";
  } catch (e) {
    fail(e);
  } finally {
    busyModel.value = false;
    load();
  }
}
const automationAsk = ref(false);
function setMode(m: string) {
  if (m === "automation") automationAsk.value = true;
  else act(Worker.SetMode(active.value!.label, m));
}
const modeHelp = computed<Record<string, string>>(() => ({
  observe: "Reads files in the workspace and runs read-only commands. No edits, no web.",
  assisted: "Also edits files in the workspace and runs its allowed commands.",
  automation: `Runs commands Claude Code's safety classifier approves, anywhere on ${words.value.thisComputer}. Deny rules and secret paths stay blocked.`,
}));
</script>

<template>
  <Page title="Sessions" subtitle="Conversations with Claude. Each one keeps its own context, workspace, tier and model.">
    <template #actions>
      <button class="btn" :disabled="!store.connection.connected" @click="open('fresh')" title="New session with the same settings; the current one is kept">Fresh context</button>
      <button class="btn" :disabled="!store.connection.connected || !active" @click="open('newf')">New &amp; forget current</button>
      <button class="btn btn-primary" :disabled="!store.connection.connected" @click="open('new')">New session</button>
    </template>
    <div v-if="!store.connection.connected" class="text-muted">Start the worker to see sessions.</div>
    <template v-else>
      <div v-if="active" class="card mb-4 px-5 py-4">
        <div class="flex items-center gap-2">
          <h2 class="font-semibold">Active: {{ active.label }}<span v-if="active.name" class="font-normal text-muted"> “{{ active.name }}”</span></h2>
          <Badge tone="accent" :label="active.mode" />
        </div>
        <div class="mt-3 grid grid-cols-3 gap-4">
          <div>
            <label class="label" for="tier">Tier</label>
            <select id="tier" class="input mt-1" :value="active.mode" @change="setMode(($event.target as HTMLSelectElement).value)">
              <option value="observe">observe</option>
              <option value="assisted">assisted</option>
              <option value="automation" :disabled="!settings?.automationEnabled">automation{{ settings?.automationEnabled ? "" : " (off in Settings)" }}</option>
            </select>
            <p class="mt-1 text-xs text-muted">{{ modeHelp[active.mode] }}</p>
          </div>
          <div>
            <label class="label" for="model">Model · now {{ active.model }}{{ active.modelSet ? "" : " (default)" }}</label>
            <div class="mt-1 flex gap-2">
              <input id="model" v-model="modelInput" class="input" list="models" placeholder="fable, opus, sonnet…" @keydown.enter="setModel" />
              <datalist id="models"><option v-for="m in models?.builtin ?? []" :key="m" :value="m" /><option v-for="a in models?.aliases ?? []" :key="a.name" :value="a.name" /></datalist>
              <button class="btn" :disabled="!modelInput || busyModel" @click="setModel">{{ busyModel ? "Checking…" : "Set" }}</button>
            </div>
            <p class="mt-1 text-xs text-muted">Each model is verified with a one-turn probe before it is saved.</p>
          </div>
          <div>
            <label class="label" for="effort">Effort · now {{ active.effort }}</label>
            <select id="effort" class="input mt-1" :value="active.effortSet ? active.effort : ''" @change="act(Worker.SetEffort(active.label, ($event.target as HTMLSelectElement).value || 'auto', false))">
              <option value="">auto (model default)</option>
              <option v-for="e in (models?.effortLevels ?? []).filter((x) => x !== 'auto')" :key="e" :value="e">{{ e }}</option>
            </select>
          </div>
        </div>
      </div>

      <div class="mb-2 flex items-center gap-4">
        <label class="flex items-center gap-2 text-muted"><input v-model="showArchived" type="checkbox" /> Show archived</label>
      </div>
      <div class="card overflow-x-auto">
        <table v-if="list.length" class="tbl">
          <thead><tr><th></th><th>Session</th><th>Workspace · tier</th><th>Model / effort</th><th>Activity</th><th>Status</th><th class="text-right">Actions</th></tr></thead>
          <tbody>
            <tr v-for="s in list" :key="s.id">
              <td><span v-if="s.active" class="text-accent" aria-label="active">●</span></td>
              <td><b>{{ s.label }}</b> <span class="text-muted">{{ s.name }}</span> <span class="mono text-muted">{{ s.claudeShort }}</span></td>
              <td><span class="mono">{{ s.workspace }}</span> · {{ s.mode }}</td>
              <td>{{ s.model }}/{{ s.effort }}</td>
              <td class="whitespace-nowrap text-xs">{{ s.turns }} turns<br /><span class="text-muted">{{ ago(s.lastUsedAt) }}</span></td>
              <td><Badge :tone="s.status === 'active' ? (s.activeJobs ? 'accent' : 'ok') : s.status === 'missing' ? 'danger' : 'muted'" :label="s.activeJobs ? `${s.activeJobs} job(s)` : s.status" /></td>
              <td class="space-x-1 text-right whitespace-nowrap">
                <button v-if="!s.active && s.status === 'active'" class="btn btn-sm" @click="act(Worker.SessionSwitch(s.label))">Switch</button>
                <button class="btn btn-sm" @click="open('rename', s)">Rename</button>
                <button v-if="s.status === 'archived'" class="btn btn-sm" @click="act(Worker.SessionRestore(s.label))">Restore</button>
                <button v-else class="btn btn-sm" :disabled="s.activeJobs > 0" @click="act(Worker.SessionArchive(s.label))">Archive</button>
                <button class="btn btn-danger btn-sm" :disabled="s.activeJobs > 0" @click="open('forget', s)">Forget…</button>
              </td>
            </tr>
          </tbody>
        </table>
        <Empty v-else text="No sessions yet. Send a message to the bot or click New session." class="m-4" />
      </div>

      <div class="mt-4">
        <button class="btn btn-ghost btn-sm" :aria-expanded="showForgotten" @click="showForgotten = !showForgotten">{{ showForgotten ? "▾" : "▸" }} Recently forgotten ({{ forgotten.length }})</button>
        <div v-if="showForgotten" class="card mt-2 overflow-hidden">
          <table v-if="forgotten.length" class="tbl">
            <thead><tr><th>Session</th><th>Forgotten</th><th>Claude Code history</th><th></th></tr></thead>
            <tbody>
              <tr v-for="s in forgotten" :key="s.id">
                <td><b>{{ s.label }}</b> <span class="text-muted">{{ s.name }}</span></td>
                <td>{{ ago(s.forgottenAt) }}</td>
                <td>{{ s.deletedAt ? "deleted" : "kept" }}</td>
                <td class="text-right"><button class="btn btn-sm" :disabled="!!s.deletedAt" @click="act(Worker.SessionRestore(s.label))">Restore</button></td>
              </tr>
            </tbody>
          </table>
          <Empty v-else text="Nothing was forgotten in the last 30 days." class="m-4" />
        </div>
      </div>
    </template>

    <Modal v-if="dlg" :title="{ new: 'New session', newf: 'New session, forget the current one', fresh: 'Fresh context', rename: 'Rename ' + dlg.s?.label, forget: 'Forget ' + dlg.s?.label }[dlg.kind]" @close="dlg = null">
      <template v-if="dlg.kind === 'new' || dlg.kind === 'newf' || dlg.kind === 'rename'">
        <label class="label" for="sname">Name (optional for new sessions)</label>
        <input id="sname" v-model="name" class="input mt-1" maxlength="32" placeholder="letters, digits, - and _" @keydown.enter="confirm" />
      </template>
      <template v-if="dlg.kind === 'new' && (settings?.workspaces?.length ?? 0) > 1">
        <label class="label mt-3 block" for="sws">Workspace</label>
        <select id="sws" v-model="workspace" class="input mt-1"><option v-for="w in settings?.workspaces ?? []" :key="w.name" :value="w.name">{{ w.name }}</option></select>
      </template>
      <p v-if="dlg.kind === 'new'" class="mt-3 text-muted">The new session becomes active; the current one stays in the list and can be resumed.</p>
      <p v-if="dlg.kind === 'fresh'" class="text-muted">Starts a new session with the same workspace, tier, model and effort as {{ active?.label ?? "the current session" }}. Nothing is deleted; the old session stays in the list.</p>
      <p v-if="dlg.kind === 'rename'" class="mt-3 text-muted">Only the label changes.</p>
      <template v-if="dlg.kind === 'newf' || dlg.kind === 'forget'">
        <p :class="dlg.kind === 'newf' ? 'mt-3' : ''">
          {{ dlg.kind === "newf" ? `A new session is created first; then ${active?.label} is forgotten from clawdyX.` : `${dlg.s?.label} disappears from the list.` }}
          Its Claude Code history is <b>kept</b> unless you tick the box below, and you can restore it within 30 days.
        </p>
        <div class="mt-3 card px-4 py-3">
          <label class="flex items-start gap-2" :class="history?.deletable ? '' : 'opacity-60'">
            <input v-model="deleteHistory" type="checkbox" :disabled="!history?.deletable" class="mt-0.5" />
            <span>
              Also delete its Claude Code history on {{ words.thisComputer }}
              <span v-if="history?.deletable" class="block text-xs text-muted">{{ history.paths?.length ?? 0 }} item(s), {{ bytes(history.bytes) }}: <span class="mono break-all">{{ (history.paths ?? []).join(", ") }}</span></span>
              <span v-else class="block text-xs text-muted">{{ history?.reason ?? "checking…" }}</span>
            </span>
          </label>
          <p v-if="deleteHistory" class="mt-2 text-danger">This cannot be undone.</p>
        </div>
      </template>
      <p v-if="err" class="mt-3 text-danger" role="alert">{{ err }}</p>
      <template #actions>
        <button class="btn" @click="dlg = null">Cancel</button>
        <button :class="deleteHistory ? 'btn btn-danger' : 'btn btn-primary'" @click="confirm">
          {{ { new: "Create", newf: deleteHistory ? "Create, forget and delete history" : "Create and forget", fresh: "Start fresh", rename: "Rename", forget: deleteHistory ? "Forget and delete history" : "Forget" }[dlg.kind] }}
        </button>
      </template>
    </Modal>

    <Modal v-if="automationAsk" title="Switch to automation?" @close="automationAsk = false">
      <p>{{ modeHelp.automation }}</p>
      <p v-if="settings?.screenControl" class="mt-2">With screen control on, Claude can also see your displays and use the mouse and keyboard.</p>
      <p class="mt-2 text-muted">Bypass mode is never used. Switch back to observe at any time.</p>
      <template #actions>
        <button class="btn" @click="automationAsk = false">Keep current tier</button>
        <button class="btn btn-primary" @click="automationAsk = false; act(Worker.SetMode(active!.label, 'automation'))">Enable automation</button>
      </template>
    </Modal>
  </Page>
</template>
