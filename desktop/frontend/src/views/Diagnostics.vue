<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import Empty from "../components/Empty.vue";
import { Worker, System, type DiagnosticCheck, type LogLine } from "../api";
import { store, toast, fail } from "../store";

const checks = ref<DiagnosticCheck[]>([]);
const running = ref(false);
const lines = ref<LogLine[]>([]);
const level = ref("info");
const q = ref("");

async function run() {
  if (!store.connection.connected) return;
  running.value = true;
  try {
    checks.value = (await Worker.Diagnostics()) ?? [];
  } catch (e) {
    fail(e);
  } finally {
    running.value = false;
  }
}
async function logs() {
  if (!store.connection.connected) return;
  lines.value = (await Worker.LogTail(2000, level.value).catch(() => [])) ?? [];
}
watch(() => store.connection.connected, () => { run(); logs(); }, { immediate: true });
watch(level, logs);
const shown = computed(() => {
  const s = q.value.toLowerCase();
  const l = s ? lines.value.filter((x) => (x.msg + " " + JSON.stringify(x.attrs ?? {})).toLowerCase().includes(s)) : lines.value;
  return l.slice(-500).reverse();
});
function fixKey(url?: string): string {
  if (!url) return "";
  if (url.includes("Privacy_ScreenCapture")) return "screen-recording";
  if (url.includes("Privacy_Accessibility")) return "accessibility";
  if (url.includes("Privacy_Camera")) return "camera";
  return "";
}
async function copyLogs() {
  try {
    await navigator.clipboard.writeText(shown.value.map((l) => `${l.time} ${l.level} ${l.msg} ${Object.entries(l.attrs ?? {}).map(([k, v]) => `${k}=${v}`).join(" ")}`).join("\n"));
    toast("Log lines copied", "ok");
  } catch (e) {
    fail(e);
  }
}
const bundle = ref<string | null>(null);
const includeIDs = ref(false);
async function prepare() {
  bundle.value = "Collecting…";
  bundle.value = await System.DiagnosticBundle(includeIDs.value);
}
async function saveBundle() {
  try {
    const p = await System.SaveDiagnostics(bundle.value ?? "");
    if (p) toast(`Saved ${p}`, "ok");
    bundle.value = null;
  } catch (e) {
    fail(e);
  }
}
</script>

<template>
  <Page title="Diagnostics" subtitle="Read-only checks and the worker's log. Nothing here changes settings.">
    <template #actions>
      <button class="btn" :disabled="running" @click="run">{{ running ? "Checking (about 30 s)…" : "Run checks" }}</button>
      <button class="btn" @click="prepare">Create report…</button>
    </template>
    <div v-if="!store.connection.connected" class="card px-5 py-4">
      <p>The worker is not reachable, so its checks cannot run. Service state: <b>{{ store.service?.pid ? "running" : store.service?.installed ? "installed but not running" : "not installed" }}</b>{{ store.service?.binary ? ` (${store.service.binary})` : "" }}.</p>
      <p class="mt-2 text-muted">Start it from Overview. If it stops right away, the log file in Settings → Logs explains why (often an invalid config.json or a missing token).</p>
    </div>
    <template v-else>
      <div class="card overflow-hidden">
        <ul v-if="checks.length" class="divide-y divide-line">
          <li v-for="c in checks" :key="c.id" class="flex items-start gap-3 px-5 py-2.5">
            <Badge :tone="c.ok ? 'ok' : c.warn ? 'warn' : 'danger'" :label="c.ok ? 'OK' : c.warn ? 'Check' : 'Problem'" class="mt-0.5 w-20 justify-center" />
            <div class="flex-1">
              <div class="font-medium">{{ c.label }}</div>
              <div class="text-muted selectable">{{ c.detail }}</div>
              <div v-if="c.fix" class="mt-0.5 text-xs">{{ c.fix }}</div>
            </div>
            <button v-if="fixKey(c.fixUrl) && !c.ok" class="btn btn-sm" @click="System.Open(fixKey(c.fixUrl))">Open System Settings</button>
          </li>
        </ul>
        <Empty v-else :text="running ? 'Running checks…' : 'No results yet.'" class="m-4" />
      </div>

      <div class="mt-6 mb-2 flex items-center gap-3">
        <h2 class="font-semibold">Log</h2>
        <select v-model="level" class="input w-32 py-1" aria-label="Minimum level"><option>debug</option><option>info</option><option>warn</option><option>error</option></select>
        <input v-model="q" class="input w-64 py-1" placeholder="Filter" aria-label="Filter log lines" />
        <button class="btn btn-sm" @click="logs">Refresh</button>
        <button class="btn btn-sm" @click="copyLogs">Copy</button>
        <span class="text-xs text-muted">newest first · redacted</span>
      </div>
      <div class="card max-h-[28rem] overflow-auto">
        <table class="tbl mono selectable">
          <tbody>
            <tr v-for="(l, i) in shown" :key="i">
              <td class="whitespace-nowrap text-muted">{{ l.time.slice(11, 19) }}</td>
              <td :class="l.level === 'ERROR' ? 'text-danger' : l.level === 'WARN' ? 'text-warn' : 'text-muted'">{{ l.level }}</td>
              <td>{{ l.msg }} <span class="text-muted">{{ Object.entries(l.attrs ?? {}).map(([k, v]) => `${k}=${v}`).join(" ") }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <Modal v-if="bundle !== null" title="Diagnostic report" wide @close="bundle = null">
      <p class="text-muted">Review everything before saving. Tokens are removed, home paths shortened to ~, and screenshots are never included.</p>
      <label class="mt-2 flex items-center gap-2"><input v-model="includeIDs" type="checkbox" @change="prepare" /> Include Telegram user IDs</label>
      <pre class="mt-3 max-h-96 overflow-auto rounded bg-surface-2 p-3 whitespace-pre-wrap mono selectable">{{ bundle }}</pre>
      <template #actions>
        <button class="btn" @click="bundle = null">Cancel</button>
        <button class="btn btn-primary" @click="saveBundle">Save…</button>
      </template>
    </Modal>
  </Page>
</template>
