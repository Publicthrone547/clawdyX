<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import WorkerDown from "../components/WorkerDown.vue";
import Empty from "../components/Empty.vue";
import { Worker, System, type JobInfo } from "../api";
import { store, toast, fail, refreshStatus, refreshService } from "../store";
import { ago, since, botTone, botLabel, jobTone } from "../util";

const st = computed(() => store.status);
const busy = ref(false);
const stopAsk = ref(false);
const active = ref<JobInfo[]>([]);

const polling = computed(() => st.value?.bot.state === "polling" || st.value?.bot.state === "paused");

async function loadJobs() {
  if (!store.connection.connected) return;
  try {
    active.value = (await Worker.Jobs(20))?.filter((j) => j.active) ?? [];
  } catch {
    active.value = [];
  }
}
watch(() => store.tick, loadJobs, { immediate: true });

async function start() {
  busy.value = true;
  try {
    await Worker.BotStart();
    toast("Bot started", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
    refreshStatus();
  }
}
async function stop(cancelJobs: boolean) {
  stopAsk.value = false;
  busy.value = true;
  try {
    const r = await Worker.BotStop(cancelJobs);
    const c = r.cancelled?.length ?? 0, k = r.running?.length ?? 0;
    toast(c ? `Bot stopped; cancelled ${r.cancelled!.join(", ")}` : k ? `Bot stopped; ${k} job(s) keep running and will report back` : "Bot stopped", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
    refreshStatus();
  }
}
function askStop() {
  if ((st.value?.jobs.running ?? 0) + (st.value?.jobs.queued ?? 0) > 0) stopAsk.value = true;
  else stop(false);
}
async function cancel(id: number) {
  try {
    await Worker.JobCancel(id);
    toast(`Cancelling j-${id}`, "ok");
  } catch (e) {
    fail(e);
  }
}
async function useAppWorker() {
  busy.value = true;
  try {
    toast((await System.InstallService()) || "The service now runs the app's worker", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
    refreshService();
  }
}
const claudeOK = computed(() => st.value?.claude.loggedIn === true);
</script>

<template>
  <Page title="Overview" subtitle="Telegram bot and Claude Code on this Mac. Closing this window keeps the bot and the menu-bar item running.">
    <template #actions>
      <template v-if="store.connection.connected">
        <button v-if="!polling" class="btn btn-primary" :disabled="busy || st?.bot.state === 'starting'" @click="start">
          {{ st?.bot.state === "starting" ? "Starting…" : "Start bot" }}
        </button>
        <button v-else class="btn" :disabled="busy || st?.bot.state === 'stopping'" @click="askStop">
          {{ st?.bot.state === "stopping" ? "Stopping…" : "Stop bot" }}
        </button>
      </template>
    </template>

    <WorkerDown v-if="!store.connection.connected" class="mb-5" />

    <div
      v-if="store.service?.inBundle && store.service?.installed && !store.service?.usesAppWorker"
      class="card mb-5 flex items-center gap-4 px-5 py-3"
    >
      <p class="flex-1 text-muted">
        The background service runs <span class="mono">{{ store.service.binary }}</span>, not the worker inside this app.
        Switching keeps your settings, sessions and token.
      </p>
      <button class="btn" :disabled="busy" @click="useAppWorker">Use the app's worker</button>
    </div>

    <div class="grid grid-cols-4 gap-3" role="list" aria-label="Status">
      <div class="card px-4 py-3" role="listitem">
        <div class="label">Worker</div>
        <div class="mt-1.5"><Badge :tone="store.connection.connected ? 'ok' : 'danger'" :label="store.connection.connected ? 'Running' : 'Not running'" /></div>
        <div class="mt-1 text-xs text-muted">{{ st ? `pid ${st.pid} · up ${since(st.startedAt)}` : store.service?.loaded ? "loaded in launchd" : "—" }}</div>
      </div>
      <div class="card px-4 py-3" role="listitem">
        <div class="label">Bot</div>
        <div class="mt-1.5"><Badge :tone="botTone(st?.bot.state)" :label="botLabel[st?.bot.state ?? 'stopped'] ?? st?.bot.state ?? '—'" /></div>
        <div class="mt-1 text-xs text-muted">{{ st?.bot.lastError || (st?.bot.since ? `since ${ago(st.bot.since)}` : "—") }}</div>
      </div>
      <div class="card px-4 py-3" role="listitem">
        <div class="label">Telegram</div>
        <div class="mt-1.5">
          <Badge v-if="!st" tone="muted" label="Unknown" />
          <Badge v-else-if="st.telegram.conflicts409 > 0" tone="danger" label="Duplicate poller" />
          <Badge v-else-if="st.telegram.botUsername" tone="ok" :label="'@' + st.telegram.botUsername" />
          <Badge v-else tone="warn" label="Not verified" />
        </div>
        <div class="mt-1 text-xs text-muted">{{ st ? `last update ${ago(st.telegram.lastUpdate)} · ${st.telegram.allowedUsers} allowed` : "—" }}</div>
      </div>
      <div class="card px-4 py-3" role="listitem">
        <div class="label">Claude Code</div>
        <div class="mt-1.5">
          <Badge v-if="!st" tone="muted" label="Unknown" />
          <Badge v-else :tone="claudeOK ? 'ok' : st.claude.loggedIn === false ? 'danger' : 'warn'" :label="claudeOK ? 'Logged in' : st.claude.loggedIn === false ? 'Not logged in' : 'Checking…'" />
        </div>
        <div class="mt-1 text-xs text-muted">{{ st ? `${st.claude.version || "?"} · ${st.claude.subscription || st.claude.authMethod || ""}` : "—" }}</div>
      </div>
    </div>

    <div v-if="st" class="mt-4 grid grid-cols-2 gap-3">
      <div class="card px-5 py-4">
        <div class="flex items-center justify-between">
          <h2 class="font-semibold">Active session</h2>
          <button class="btn btn-sm" @click="store.view = 'sessions'">Sessions…</button>
        </div>
        <div v-if="st.active" class="mt-3 space-y-1">
          <div class="text-base font-semibold">{{ st.active.label }}<span v-if="st.active.name" class="font-normal text-muted"> “{{ st.active.name }}”</span></div>
          <div class="text-muted">
            workspace <span class="mono">{{ st.active.workspace }}</span> · tier <b>{{ st.active.mode }}</b> · {{ st.active.model }}/{{ st.active.effort }}
          </div>
          <div class="text-xs text-muted">{{ st.active.turns }} turns · last used {{ ago(st.active.lastUsedAt) }} · Claude session {{ st.active.claudeShort }}</div>
        </div>
        <p v-else class="mt-3 text-muted">No active session. The next message to the bot starts one.</p>
      </div>
      <div class="card px-5 py-4">
        <div class="flex items-center justify-between">
          <h2 class="font-semibold">Jobs</h2>
          <button class="btn btn-sm" @click="store.view = 'jobs'">Jobs…</button>
        </div>
        <p class="mt-1 text-xs text-muted">{{ st.jobs.running }} running · {{ st.jobs.queued }} queued · at most {{ st.jobs.maxConcurrent }} at once</p>
        <ul v-if="active.length" class="mt-3 space-y-2">
          <li v-for="j in active" :key="j.id" class="flex items-center gap-2">
            <Badge :tone="jobTone(j.state)" :label="j.state" />
            <span class="font-medium">{{ j.label }}</span>
            <span class="flex-1 truncate text-xs text-muted">S{{ j.sessionId }} · {{ store.progress[j.id]?.lastTool || j.model }}</span>
            <button class="btn btn-sm" @click="cancel(j.id)">Cancel</button>
          </li>
        </ul>
        <Empty v-else class="mt-3" text="Nothing is running." />
      </div>
    </div>

    <div v-if="st?.lastError" class="card mt-4 px-5 py-3">
      <div class="label">Last error · {{ ago(st.lastErrorAt) }}</div>
      <p class="mt-1 selectable">{{ st.lastError }}</p>
    </div>
    <div v-if="st && st.telegram.webhookRemoved" class="card mt-4 px-5 py-3" style="border-color: var(--danger)">
      A webhook was configured for this bot and was removed at start. If you did not set it, the token may have leaked: replace it on the Telegram page and revoke the old one in @BotFather.
    </div>

    <Modal v-if="stopAsk" title="Stop the bot?" @close="stopAsk = false">
      <p>{{ (st?.jobs.running ?? 0) + (st?.jobs.queued ?? 0) }} job(s) are running or queued. The worker keeps running either way; only Telegram polling stops.</p>
      <template #actions>
        <button class="btn" @click="stopAsk = false">Cancel</button>
        <button class="btn" @click="stop(false)">Stop polling, let jobs finish</button>
        <button class="btn btn-danger" @click="stop(true)">Stop and cancel jobs</button>
      </template>
    </Modal>
  </Page>
</template>
