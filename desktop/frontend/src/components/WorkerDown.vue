<script setup lang="ts">
import { ref } from "vue";
import { System } from "../api";
import { store, toast, fail, refreshService } from "../store";
const busy = ref(false);
async function start() {
  busy.value = true;
  try {
    const out = store.service?.installed ? await System.StartService() : await System.InstallService();
    toast(out || "Worker started", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
    refreshService();
  }
}
</script>

<template>
  <div class="card flex items-start gap-4 px-5 py-4" style="border-color: var(--warn)">
    <div class="flex-1">
      <h2 class="font-semibold">The background worker is not running</h2>
      <p class="mt-1 text-muted">
        The worker runs the Telegram bot and Claude jobs. It starts at login and keeps running when this window is closed.
        <span v-if="store.service?.installed">It is installed but not running{{ store.service?.binary ? ` (${store.service.binary})` : "" }}.</span>
        <span v-else>It is not installed yet.</span>
      </p>
    </div>
    <div class="flex gap-2">
      <button class="btn btn-primary" :disabled="busy" @click="start">{{ busy ? "Starting…" : store.service?.installed ? "Start worker" : "Install and start" }}</button>
      <button class="btn" @click="store.view = 'diagnostics'">Diagnostics</button>
    </div>
  </div>
</template>
