<script setup lang="ts">
import { onMounted, ref } from "vue";
import Badge from "../../components/Badge.vue";
import { System, type D } from "../../api";
import { store, toast, fail } from "../../store";
import { words } from "../../platform";

const app = ref<D.AppInfo | null>(null);
const update = ref<D.UpdateInfo | null>(null);
const checking = ref(false);
onMounted(async () => (app.value = await System.Info().catch(() => null)));

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
// "Windows 11 23H2 · amd64", "Ubuntu 24.04 LTS · amd64 · Wayland (GNOME)".
function systemLine(): string {
  const p = store.platform;
  if (!p) return "—";
  let line = (p.pretty || p.osName) + " · " + p.arch;
  if (p.os === "linux") line += " · " + (p.headless ? "no graphical session" : p.session === "wayland" ? "Wayland" : "X11") + (p.desktop ? " (" + p.desktop + ")" : "");
  return line;
}
const busy = ref(false);
async function restart() {
  busy.value = true;
  try {
    toast((await System.RestartService()) || "Restarted", "ok");
  } catch (e) {
    fail(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
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
      <dt class="text-muted">System</dt><dd>{{ systemLine() }}</dd>
    </dl>
    <div v-if="update?.newer" class="mt-3 rounded-md border border-line bg-surface-2 px-3 py-2">
      <b>clawdyX {{ update.latest }} is available.</b> {{ words.updateHint }}
    </div>
    <div class="mt-3 flex flex-wrap gap-2">
      <button v-if="update?.newer" class="btn btn-primary" @click="System.OpenRelease(update.latest).catch(fail)">Get {{ update.latest }}</button>
      <button class="btn" :disabled="checking" @click="checkUpdate">{{ checking ? "Checking…" : "Check for updates" }}</button>
      <button v-if="app?.version" class="btn" @click="System.OpenRelease('').catch(fail)">What's new in {{ app.version }}</button>
      <button class="btn" @click="System.Open('releases').catch(fail)">All releases</button>
      <button v-if="workerOutdated()" class="btn" :disabled="busy" @click="restart">Restart worker</button>
    </div>
    <p class="mt-2 text-xs text-muted">Checking for updates asks GitHub for the latest release, only when you click. clawdyX never checks on its own.</p>
  </section>
</template>
