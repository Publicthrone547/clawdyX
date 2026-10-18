<script setup lang="ts">
import { onMounted, ref } from "vue";
import Badge from "../../components/Badge.vue";
import Modal from "../../components/Modal.vue";
import { System, type D } from "../../api";
import { store, toast, fail, refreshService } from "../../store";
import { words } from "../../platform";

const app = ref<D.AppInfo | null>(null);
onMounted(async () => {
  app.value = await System.Info().catch(() => null);
  refreshService();
});
const busy = ref(false);
async function svc(fn: () => Promise<string>) {
  busy.value = true;
  try {
    toast((await fn()) || "Done", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
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
  <section class="card mt-4 px-5 py-4">
    <h2 class="font-semibold">Background worker</h2>
    <p class="text-xs text-muted">Runs the Telegram bot as a {{ words.serviceKind }}, also while this window is closed.</p>
    <dl class="mt-3 grid grid-cols-[10rem_1fr] gap-y-1.5">
      <dt class="text-muted">Service</dt><dd><Badge :tone="store.service?.pid ? 'ok' : store.service?.installed ? 'warn' : 'muted'" :label="store.service?.pid ? `running (pid ${store.service.pid})` : store.service?.installed ? 'installed, not running' : 'not installed'" /></dd>
      <dt class="text-muted">Program</dt><dd class="mono">{{ store.service?.binary || "—" }}</dd>
      <dt class="text-muted">Settings file</dt><dd class="mono">{{ app?.configPath }}</dd>
    </dl>
    <div class="mt-3 flex flex-wrap gap-2">
      <button v-if="store.service?.inBundle && !store.service?.usesAppWorker" class="btn" :disabled="busy" @click="svc(System.InstallService)">Use the app's worker</button>
      <button class="btn" :disabled="busy" @click="svc(System.RestartService)">Restart</button>
      <button class="btn" :disabled="busy || !store.service?.loaded" @click="svc(System.StopService)">Stop</button>
      <button class="btn" :disabled="busy || !!store.service?.pid" @click="svc(store.service?.installed ? System.StartService : System.InstallService)">Start</button>
      <button class="btn btn-danger" :disabled="busy || !store.service?.installed" @click="uninstallAsk = true">Uninstall…</button>
    </div>
    <Modal v-if="uninstallAsk" title="Uninstall the background worker?" @close="uninstallAsk = false">
      <p>The worker stops and no longer starts at login. Claude Code, its login and your project folders are never touched.</p>
      <label class="mt-3 flex items-center gap-2"><input v-model="purgeState" type="checkbox" /> Also delete clawdyX's state (sessions, jobs, audit log) and logs</label>
      <p class="mt-2 text-xs text-muted">The settings and the token in {{ words.tokenStore }} are kept so you can reinstall.</p>
      <template #actions>
        <button class="btn" @click="uninstallAsk = false">Cancel</button>
        <button class="btn btn-danger" @click="doUninstall">Uninstall</button>
      </template>
    </Modal>
  </section>
</template>
