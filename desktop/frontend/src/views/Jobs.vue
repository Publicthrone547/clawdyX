<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import Empty from "../components/Empty.vue";
import { Worker, type JobInfo, type JobEvent, type ProcessesInfo } from "../api";
import { store, toast, fail } from "../store";
import { ago, dur, jobTone, bytes } from "../util";

const jobs = ref<JobInfo[]>([]);
const procs = ref<ProcessesInfo | null>(null);
const loadingProcs = ref(false);

async function loadJobs() {
  if (!store.connection.connected) return;
  try {
    jobs.value = (await Worker.Jobs(50)) ?? [];
  } catch (e) {
    fail(e);
  }
}
async function loadProcs() {
  if (!store.connection.connected) return;
  loadingProcs.value = true;
  try {
    procs.value = await Worker.Processes();
  } catch (e) {
    fail(e);
  } finally {
    loadingProcs.value = false;
  }
}
watch(() => store.tick, loadJobs, { immediate: true });
watch(() => store.connection.connected, loadProcs, { immediate: true });
const timer = setInterval(() => document.visibilityState === "visible" && loadProcs(), 10000);
onBeforeUnmount(() => clearInterval(timer));

async function cancel(id: number) {
  try {
    const r = await Worker.JobCancel(id);
    toast(`Cancelling j-${id} (was ${r.previous})`, "ok");
  } catch (e) {
    fail(e);
  }
}

const detail = ref<JobInfo | null>(null);
const events = ref<JobEvent[]>([]);
async function show(j: JobInfo) {
  detail.value = j;
  events.value = (await Worker.JobEvents(j.id, 200).catch(() => [])) ?? [];
}
const icons: Record<string, string> = { tool_use: "🔧", denial: "🚫", api_retry: "↻" };
</script>

<template>
  <Page title="Jobs & processes" subtitle="Prompts the bot ran, the Claude processes it owns, and other Claude Code sessions on this Mac">
    <template #actions>
      <button class="btn" :disabled="loadingProcs" @click="loadJobs(); loadProcs()">Refresh</button>
    </template>
    <div v-if="!store.connection.connected" class="text-muted">Start the worker to see jobs.</div>
    <template v-else>
      <h2 class="mb-2 font-semibold">Owned by clawdyX</h2>
      <div class="card overflow-hidden">
        <table v-if="procs?.owned?.length" class="tbl">
          <thead><tr><th>Job</th><th>Session</th><th>State</th><th>PID</th><th>Running for</th><th>Memory</th><th>CPU</th><th>Model · tier</th><th></th></tr></thead>
          <tbody>
            <tr v-for="p in procs.owned" :key="p.jobId">
              <td><b>j-{{ p.jobId }}</b></td>
              <td>{{ p.session }} <span class="text-muted">{{ p.workspace }}</span></td>
              <td><Badge :tone="jobTone(p.state)" :label="p.state" /></td>
              <td class="mono">{{ p.pid || "—" }}</td>
              <td>{{ p.started ? dur(Date.now() - p.started * 1000) : "—" }}</td>
              <td>{{ p.rssKB ? bytes(p.rssKB * 1024) : "—" }}</td>
              <td>{{ p.cpu >= 0 ? p.cpu.toFixed(0) + " %" : "—" }}</td>
              <td>{{ p.model }} · {{ p.mode }}</td>
              <td class="text-right"><button class="btn btn-sm" @click="cancel(p.jobId)">Cancel</button></td>
            </tr>
          </tbody>
        </table>
        <Empty v-else text="No Claude process is running for the bot." class="m-4" />
      </div>
      <p class="mt-1 text-xs text-muted">CPU is measured over one second. Only processes the bot started can be cancelled here. Worker memory: {{ procs ? bytes(procs.workerRssKB * 1024) : "—" }}.</p>

      <h2 class="mt-6 mb-2 font-semibold">Recent jobs</h2>
      <div class="card overflow-hidden">
        <table v-if="jobs.length" class="tbl">
          <thead><tr><th>Job</th><th>Session</th><th>State</th><th>Progress</th><th>Created</th><th>Tokens in / out</th><th></th></tr></thead>
          <tbody>
            <tr v-for="j in jobs" :key="j.id">
              <td><b>{{ j.label }}</b> <span v-if="j.kind === 'look'" class="text-xs text-muted">screen</span></td>
              <td>S{{ j.sessionId }}</td>
              <td><Badge :tone="jobTone(j.state)" :label="j.state" /></td>
              <td class="max-w-72 truncate text-xs text-muted">
                <template v-if="store.progress[j.id]">{{ dur(store.progress[j.id].elapsedMs) }} · {{ store.progress[j.id].tools }} tool calls · {{ store.progress[j.id].lastTool }}</template>
                <template v-else-if="j.errorSummary">{{ j.errorSummary }}</template>
                <template v-else-if="j.finishedAt && j.startedAt">{{ dur((j.finishedAt - j.startedAt) * 1000) }} · {{ j.numTurns }} turns</template>
              </td>
              <td>{{ ago(j.createdAt) }}</td>
              <td>{{ j.inTokens }} / {{ j.outTokens }}</td>
              <td class="space-x-1 text-right whitespace-nowrap">
                <button class="btn btn-sm" @click="show(j)">Details</button>
                <button v-if="j.active" class="btn btn-sm" @click="cancel(j.id)">Cancel</button>
              </td>
            </tr>
          </tbody>
        </table>
        <Empty v-else text="No jobs yet." class="m-4" />
      </div>

      <h2 class="mt-6 mb-2 font-semibold">Claude Code sessions on this Mac</h2>
      <div class="card overflow-hidden">
        <table v-if="procs?.agents?.length" class="tbl">
          <thead><tr><th>Kind</th><th>Name</th><th>Status</th><th>PID</th><th>Since</th><th>Folder</th><th>Owner</th></tr></thead>
          <tbody>
            <tr v-for="(a, i) in procs.agents" :key="i">
              <td>{{ a.kind }}</td><td>{{ a.name }}</td><td>{{ a.status }}</td><td class="mono">{{ a.pid || "—" }}</td>
              <td>{{ ago(a.started) }}</td><td class="mono">{{ a.dir }}</td>
              <td><Badge :tone="a.ownedBy ? 'accent' : 'muted'" :label="a.ownedBy ? 'clawdyX ' + a.ownedBy : 'other (not controllable here)'" /></td>
            </tr>
          </tbody>
        </table>
        <Empty v-else :text="procs?.agentsError || 'None reported.'" class="m-4" />
      </div>
      <p class="mt-1 text-xs text-muted">
        From <span class="mono">claude agents --json</span>, refreshed every minute ({{ procs?.agentsAt ? ago(procs.agentsAt) : "not yet" }}).
        <template v-if="procs?.system">System load {{ procs.system.load1.toFixed(2) }} · {{ bytes(procs.system.memFree) }} of {{ bytes(procs.system.memTotal) }} free or reclaimable.</template>
      </p>
    </template>

    <Modal v-if="detail" :title="`${detail.label} · ${detail.state}`" wide @close="detail = null">
      <dl class="grid grid-cols-[9rem_1fr] gap-y-1 text-sm">
        <dt class="text-muted">Session</dt><dd>S{{ detail.sessionId }}</dd>
        <dt class="text-muted">Model / effort</dt><dd>{{ detail.resolvedModel || detail.model }} / {{ detail.effort }}</dd>
        <dt class="text-muted">Tier</dt><dd>{{ detail.mode }}</dd>
        <dt class="text-muted">Turns · tokens</dt><dd>{{ detail.numTurns }} · in {{ detail.inTokens }} / out {{ detail.outTokens }}</dd>
      </dl>
      <h3 class="mt-4 label">Activity</h3>
      <ul v-if="events.length" class="mt-1 max-h-60 space-y-0.5 overflow-auto mono selectable">
        <li v-for="(e, i) in events" :key="i">{{ new Date(e.ts * 1000).toLocaleTimeString() }} {{ icons[e.type] ?? "·" }} {{ e.payload }}</li>
      </ul>
      <p v-else class="mt-1 text-muted">No tool activity recorded.</p>
      <template v-if="detail.errorSummary">
        <h3 class="mt-4 label">Error</h3>
        <p class="mt-1 text-danger selectable">{{ detail.errorSummary }}</p>
      </template>
      <h3 class="mt-4 label">Result (first 2 KB)</h3>
      <pre class="mt-1 max-h-72 overflow-auto rounded bg-surface-2 p-3 whitespace-pre-wrap mono selectable">{{ store.finished[detail.id]?.text || detail.resultPreview || "—" }}</pre>
    </Modal>
  </Page>
</template>
