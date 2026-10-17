<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import Empty from "../components/Empty.vue";
import { Worker, System, type ScreensInfo, type CaptureInfo, type SessionInfo } from "../api";
import { store, toast, fail } from "../store";
import { bytes, dataURL } from "../util";

const info = ref<ScreensInfo | null>(null);
const caps = ref<CaptureInfo[]>([]);
const busy = ref("");
const loading = ref(false);

async function load() {
  if (!store.connection.connected) return;
  loading.value = true;
  try {
    info.value = await Worker.Screens();
  } catch (e) {
    fail(e);
  } finally {
    loading.value = false;
  }
}
watch(() => store.connection.connected, load, { immediate: true });

const enabled = computed(() => !!info.value?.status.enabled);
const configured = computed(() => !!info.value?.status.configured);

async function capture(display: string, label: string) {
  busy.value = label;
  try {
    const r = await Worker.Capture(display);
    caps.value = [...(r.captures ?? []), ...caps.value].slice(0, 6);
    if (r.error) toast(r.error, "danger");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = "";
    load();
  }
}
async function discard(c: CaptureInfo) {
  caps.value = caps.value.filter((x) => x.id !== c.id);
  await Worker.CaptureDiscard(c.id).catch(() => {});
}
async function toggle() {
  try {
    await Worker.SetScreenEnabled(!enabled.value);
    if (enabled.value) caps.value = [];
    await load();
  } catch (e) {
    fail(e);
  }
}

// preview
const preview = ref<{ c: CaptureInfo; full: string } | null>(null);
async function open(c: CaptureInfo) {
  preview.value = { c, full: "" };
  try {
    const f = await Worker.CaptureFull(c.id);
    if (preview.value?.c.id === c.id) preview.value.full = dataURL(f.jpeg as unknown as string);
  } catch (e) {
    fail(e);
  }
}

// use in a task
const use = ref<CaptureInfo | null>(null);
const sessions = ref<SessionInfo[]>([]);
const target = ref("");
const prompt = ref("");
const sentJob = ref<number | null>(null);
async function openUse(c: CaptureInfo) {
  use.value = c;
  prompt.value = "";
  sentJob.value = null;
  sessions.value = (await Worker.Sessions(false).catch(() => [])) ?? [];
  target.value = sessions.value.find((s) => s.active)?.label ?? sessions.value[0]?.label ?? "";
}
async function send() {
  if (!use.value) return;
  try {
    const r = await Worker.CaptureUse(use.value.id, target.value, prompt.value);
    sentJob.value = r.job.id;
    toast(`Sent to Claude as ${r.job.label}`, "ok");
  } catch (e) {
    fail(e);
  }
}
const result = computed(() => (sentJob.value ? store.finished[sentJob.value] : undefined));
const live = computed(() => (sentJob.value ? store.progress[sentJob.value] : undefined));
</script>

<template>
  <Page title="Monitors" subtitle="Your displays. Nothing is captured unless you click Capture.">
    <template #actions>
      <button class="btn" :disabled="loading" @click="load">Refresh list</button>
      <button class="btn btn-primary" :disabled="!enabled || !!busy || !info?.displays?.length" @click="capture('all', 'all displays')">Capture all displays</button>
    </template>
    <div v-if="!store.connection.connected" class="text-muted">Start the worker to use screen features.</div>
    <template v-else-if="info">
      <div class="card mb-4 flex items-center gap-4 px-5 py-3">
        <div class="flex-1">
          <div class="flex items-center gap-2">
            <span class="font-semibold">Screen features</span>
            <Badge :tone="enabled ? 'ok' : 'muted'" :label="!configured ? 'off in config' : enabled ? 'on' : 'switched off'" />
            <Badge v-if="store.capturing" tone="accent" :label="'Capturing ' + store.capturing" />
          </div>
          <p class="mt-0.5 text-xs text-muted">
            The switch covers every capture path: this window, Telegram's /screen and /look, and Claude's screenshot tool. {{ info.capturesToday }} capture(s) today; {{ info.held }} held in memory (kept at most 5 minutes).
          </p>
        </div>
        <button v-if="configured" class="btn" :class="enabled ? 'btn-danger' : ''" @click="toggle">{{ enabled ? "Switch off" : "Switch on" }}</button>
        <button v-else class="btn" @click="store.view = 'settings'">Settings…</button>
      </div>

      <div v-if="enabled && info.access !== 'granted'" class="card mb-4 flex items-center gap-4 px-5 py-3" :style="info.access === 'denied' ? 'border-color: var(--danger)' : ''">
        <p class="flex-1">
          <template v-if="info.access === 'denied'">macOS has not granted <b>Screen Recording</b> to the clawdyX worker. Enable it in System Settings → Privacy &amp; Security → Screen &amp; System Audio Recording, then restart the worker.</template>
          <template v-else>Screen Recording permission is not known yet; it is checked on the first capture.</template>
        </p>
        <button class="btn" @click="System.RequestScreenAccess()">Ask macOS</button>
        <button class="btn" @click="System.Open('screen-recording')">Open System Settings</button>
        <button class="btn btn-ghost" @click="load">Re-check</button>
      </div>

      <div class="grid grid-cols-3 gap-3">
        <div v-for="d in info.displays" :key="d.key" class="card px-4 py-3">
          <div class="flex items-center gap-2">
            <span class="font-semibold">{{ d.index }} · {{ d.name }}</span>
            <Badge v-if="d.main" tone="accent" label="main" />
            <Badge v-if="d.builtin" tone="muted" label="built-in" />
          </div>
          <div class="mt-1 text-xs text-muted">{{ d.pixelW }}×{{ d.pixelH }} px · {{ d.pointW }}×{{ d.pointH }} pt · scale {{ d.scale }} · <span class="mono">{{ d.key.slice(0, 8) }}</span></div>
          <button class="btn btn-sm mt-3" :disabled="!enabled || !!busy" @click="capture(String(d.index), d.name)">{{ busy === d.name ? "Capturing…" : "Capture this display" }}</button>
        </div>
      </div>
      <Empty v-if="!info.displays?.length" :text="info.error || 'No display reported.'" class="mt-2" />

      <h2 v-if="caps.length" class="mt-6 mb-2 font-semibold">Captures (in memory only)</h2>
      <div class="grid grid-cols-3 gap-3">
        <figure v-for="c in caps" :key="c.id" class="card overflow-hidden">
          <button class="block w-full" :aria-label="`Open capture of ${c.display.name}`" @click="open(c)"><img :src="dataURL(c.preview as unknown as string)" alt="" class="aspect-video w-full object-cover" /></button>
          <figcaption class="px-3 py-2">
            <div class="text-sm font-medium">{{ c.display.index }} · {{ c.display.name }}</div>
            <div class="text-xs text-muted">{{ new Date(c.takenAtMs).toLocaleTimeString() }} · {{ c.fullW }}×{{ c.fullH }} · {{ bytes(c.bytes) }}</div>
            <div class="mt-2 flex gap-2">
              <button class="btn btn-sm btn-primary" :disabled="!enabled" @click="openUse(c)">Use in a Claude task…</button>
              <button class="btn btn-sm" @click="discard(c)">Discard</button>
            </div>
          </figcaption>
        </figure>
      </div>
      <p class="mt-6 text-xs text-muted">macOS asks about once a month whether clawdyX may keep capturing without the system picker. The dialog appears on this Mac; until it is answered, captures (including from Telegram) fail. Audio is never recorded.</p>
    </template>

    <Modal v-if="preview" :title="`${preview.c.display.index} · ${preview.c.display.name}`" wide @close="preview = null">
      <img v-if="preview.full" :src="preview.full" :alt="`Screenshot of ${preview.c.display.name}`" class="w-full rounded" />
      <p v-else class="text-muted">Loading full resolution…</p>
    </Modal>

    <Modal v-if="use" title="Send this capture to Claude" @close="use = null">
      <img :src="dataURL(use.preview as unknown as string)" alt="" class="mb-3 w-full rounded" />
      <p class="text-muted">The image goes to Claude as part of the prompt, through Claude Code's normal request flow. It is not sent to Telegram and not stored.</p>
      <label class="label mt-3 block" for="usess">Session</label>
      <select id="usess" v-model="target" class="input mt-1"><option v-for="s in sessions" :key="s.id" :value="s.label">{{ s.label }} {{ s.name }} · {{ s.mode }}</option></select>
      <label class="label mt-3 block" for="uprompt">Question (optional)</label>
      <textarea id="uprompt" v-model="prompt" class="input mt-1" rows="3" placeholder="Describe what is on this screen and point out anything that looks wrong."></textarea>
      <div v-if="sentJob" class="mt-3 card px-3 py-2">
        <div v-if="!result" class="text-muted">j-{{ sentJob }} {{ live ? `running · ${live.tools} tool calls` : "queued…" }}</div>
        <div v-else>
          <Badge :tone="result.state === 'completed' ? 'ok' : 'danger'" :label="result.state" />
          <pre class="mt-2 max-h-64 overflow-auto whitespace-pre-wrap mono selectable">{{ result.text || result.error }}</pre>
        </div>
      </div>
      <template #actions>
        <button class="btn" @click="use = null">Close</button>
        <button class="btn btn-primary" :disabled="!!sentJob || !target" @click="send">Send to Claude</button>
      </template>
    </Modal>
  </Page>
</template>
