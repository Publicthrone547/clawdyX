<script setup lang="ts">
import { ref, watch } from "vue";
import Page from "../components/Page.vue";
import Modal from "../components/Modal.vue";
import About from "./settings/About.vue";
import General from "./settings/General.vue";
import ClaudeCode from "./settings/ClaudeCode.vue";
import Defaults from "./settings/Defaults.vue";
import Workspaces from "./settings/Workspaces.vue";
import Limits from "./settings/Limits.vue";
import Features from "./settings/Features.vue";
import Permissions from "./settings/Permissions.vue";
import Logs from "./settings/Logs.vue";
import Advanced from "./settings/Advanced.vue";
import Worker from "./settings/Worker.vue";
import { Worker as W, System } from "../api";
import { store, toast, fail, refreshService } from "../store";
import { words } from "../platform";
import { st, load, save, changes, dirty } from "./settings/state";

watch(() => store.connection.connected, load, { immediate: true });

// Turning on automation, the screen features or the camera asks first.
const riskAsk = ref(false);
function onSave() {
  const p = changes();
  const s = st.s;
  if ((p.automationEnabled && !s?.automationEnabled) || (p.screenControl && !s?.screenControl) || (p.camera && !s?.camera)) riskAsk.value = true;
  else save();
}
function confirmSave() {
  riskAsk.value = false;
  save();
}
async function restartWorker() {
  try {
    await W.RestartWorker();
    toast("Restarting the worker…", "ok");
  } catch (e) {
    fail(e);
  }
}
const starting = ref(false);
async function startWorker() {
  starting.value = true;
  try {
    toast((await (store.service?.installed ? System.StartService() : System.InstallService())) || "Started", "ok");
  } catch (e) {
    fail(e);
  } finally {
    starting.value = false;
    refreshService();
  }
}
</script>

<template>
  <Page title="Settings" subtitle="Most changes apply after the worker restarts.">
    <template #actions>
      <button class="btn btn-primary" :disabled="st.saving || !dirty()" @click="onSave">Save</button>
    </template>
    <div v-if="st.s?.restartPending" class="card mb-4 flex items-center gap-4 px-5 py-3" style="border-color: var(--warn)">
      <p class="flex-1">Saved changes take effect after the worker restarts. Running jobs are interrupted, not re-run.</p>
      <button class="btn" @click="restartWorker">Restart worker</button>
    </div>
    <div v-if="st.offline && st.s" class="card mb-4 px-5 py-3" style="border-color: var(--warn)">
      <div class="flex items-center gap-4">
        <p class="flex-1">The worker is not running; changes are saved directly and apply when it starts.</p>
        <button class="btn" :disabled="starting" @click="startWorker">Start worker</button>
      </div>
      <p v-if="st.s.invalid" class="mt-2 text-sm" style="color: var(--danger)">The worker cannot start with these settings: <span class="mono whitespace-pre-wrap">{{ st.s.invalid }}</span></p>
    </div>

    <About />
    <General />
    <template v-if="st.s">
      <ClaudeCode />
      <Defaults />
      <Workspaces />
      <Limits />
      <Features />
      <Permissions />
      <Logs />
      <Advanced />
    </template>
    <p v-else-if="!st.loading" class="card mt-4 px-5 py-4 text-muted">The settings could not be read. Set up clawdyX first, or check Diagnostics.</p>
    <Worker />

    <Modal v-if="riskAsk" title="Turn on a powerful feature?" @close="riskAsk = false">
      <p v-if="changes().automationEnabled">Automation lets Claude run shell commands anywhere on {{ words.thisComputer }} when Claude Code's classifier approves them. Deny rules and secret paths stay blocked, and each session still needs an explicit switch.</p>
      <p v-if="changes().screenControl" class="mt-2">Screen features let you (and, in automation sessions, Claude) capture your displays. {{ words.screenRiskNote }}</p>
      <p v-if="changes().camera" class="mt-2">The camera lets you get a photo or a short clip from the camera of {{ words.thisComputer }} in your private Telegram chat. Only you can ask; Claude cannot. {{ words.cameraRiskNote }}</p>
      <template #actions>
        <button class="btn" @click="riskAsk = false">Cancel</button>
        <button class="btn btn-primary" @click="confirmSave">Turn on and save</button>
      </template>
    </Modal>
  </Page>
</template>
