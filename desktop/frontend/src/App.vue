<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, defineAsyncComponent } from "vue";
import { store, initStore, refreshStatus, type View } from "./store";
import Icon from "./components/Icon.vue";
import Toasts from "./components/Toasts.vue";
import Badge from "./components/Badge.vue";
import Overview from "./views/Overview.vue";
import { botTone, botLabel } from "./util";

const Telegram = defineAsyncComponent(() => import("./views/Telegram.vue"));
const Sessions = defineAsyncComponent(() => import("./views/Sessions.vue"));
const Jobs = defineAsyncComponent(() => import("./views/Jobs.vue"));
const Monitors = defineAsyncComponent(() => import("./views/Monitors.vue"));
const Settings = defineAsyncComponent(() => import("./views/Settings.vue"));
const Diagnostics = defineAsyncComponent(() => import("./views/Diagnostics.vue"));
const Wizard = defineAsyncComponent(() => import("./views/Wizard.vue"));

const nav: { id: View; label: string; key: string }[] = [
  { id: "overview", label: "Overview", key: "1" },
  { id: "telegram", label: "Telegram", key: "2" },
  { id: "sessions", label: "Sessions", key: "3" },
  { id: "jobs", label: "Jobs & processes", key: "4" },
  { id: "monitors", label: "Monitors", key: "5" },
  { id: "settings", label: "Settings", key: "6" },
  { id: "diagnostics", label: "Diagnostics", key: "7" },
];
const views: Record<View, unknown> = { overview: Overview, telegram: Telegram, sessions: Sessions, jobs: Jobs, monitors: Monitors, settings: Settings, diagnostics: Diagnostics };

const needsSetup = computed(() => store.ready && store.setup !== null && (!store.setup.configExists || !store.setup.tokenStored));
const botState = computed(() => (store.connection.connected ? store.status?.bot.state : undefined));

function onKey(e: KeyboardEvent) {
  if (!e.metaKey || e.altKey || e.ctrlKey) return;
  const n = nav.find((x) => x.key === e.key);
  if (n) {
    store.view = n.id;
    e.preventDefault();
  } else if (e.key === "r") {
    refreshStatus();
    store.tick++;
    e.preventDefault();
  } else if (e.key === ",") {
    store.view = "settings";
    e.preventDefault();
  }
}
onMounted(() => {
  initStore();
  window.addEventListener("keydown", onKey);
});
onBeforeUnmount(() => window.removeEventListener("keydown", onKey));
</script>

<template>
  <div class="flex h-full">
    <nav class="drag flex w-56 shrink-0 flex-col border-r border-line bg-surface pt-12 pb-4" aria-label="Main">
      <div class="px-4 pb-4">
        <div class="flex items-baseline gap-2">
          <span class="text-base font-semibold tracking-tight">clawdyX</span>
          <button v-if="store.appVersion" class="nodrag text-xs text-muted hover:underline" title="Version and updates (Settings)" @click="store.view = 'settings'">v{{ store.appVersion }}</button>
        </div>
        <div class="mt-1">
          <Badge v-if="!store.connection.connected" tone="danger" label="Worker not running" />
          <Badge v-else :tone="botTone(botState)" :label="'Bot ' + (botLabel[botState ?? 'stopped'] ?? botState).toLowerCase()" />
        </div>
      </div>
      <ul class="nodrag flex flex-col gap-0.5 px-2">
        <li v-for="n in nav" :key="n.id">
          <button
            class="flex w-full items-center gap-3 rounded-md px-3 py-1.5 text-left"
            :class="store.view === n.id ? 'bg-surface-2 font-semibold' : 'text-muted hover:bg-surface-2'"
            :aria-current="store.view === n.id ? 'page' : undefined"
            :title="`${n.label} (⌘${n.key})`"
            @click="store.view = n.id"
          >
            <Icon :name="n.id" :size="16" />
            <span class="flex-1">{{ n.label }}</span>
            <span v-if="n.id === 'monitors' && store.capturing" class="text-xs text-accent" aria-label="capturing">●</span>
            <span v-if="n.id === 'jobs' && (store.status?.jobs.running ?? 0) > 0" class="rounded-full bg-accent px-1.5 text-[10px] text-accent-fg">{{ store.status?.jobs.running }}</span>
          </button>
        </li>
      </ul>
      <div class="mt-auto px-4 text-xs text-muted">⌘1–7 switch · ⌘R refresh</div>
    </nav>
    <main class="min-w-0 flex-1">
      <div v-if="!store.ready" class="flex h-full items-center justify-center text-muted">Loading…</div>
      <component :is="views[store.view]" v-else />
    </main>
    <Wizard v-if="needsSetup" />
    <Toasts />
  </div>
</template>
