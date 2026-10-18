<script setup lang="ts">
import { onMounted, ref, watch } from "vue";
import Section from "./Section.vue";
import { Worker, type ModelsInfo } from "../../api";
import { store, toast, fail } from "../../store";
import { st, fill } from "./state";

// Model, effort and the default workspace apply at once (the worker keeps
// them); they need the worker.
const models = ref<ModelsInfo | null>(null);
const model = ref("");
async function load() {
  models.value = store.connection.connected ? await Worker.Models().catch(() => null) : null;
}
onMounted(load);
watch(() => store.connection.connected, load);
async function set(kind: "model" | "effort", v: string) {
  try {
    const r = kind === "model" ? await Worker.SetModel("", v, true) : await Worker.SetEffort("", v, true);
    toast(r.message, "ok");
    await load();
  } catch (e) {
    fail(e);
  }
}
async function setWorkspace(name: string) {
  try {
    fill(await Worker.SetDefaultWorkspace(name));
    toast(`New sessions will use ${name}.`, "ok");
  } catch (e) {
    fail(e);
  }
}
</script>

<template>
  <Section title="Claude defaults for new sessions" section="defaults" hint="These apply at once.">
    <p v-if="st.offline" class="mt-2 text-xs text-muted">Start the worker to change the defaults (it checks the model with Claude Code first).</p>
    <div class="mt-3 grid grid-cols-3 gap-4">
      <div>
        <label class="label" for="dmodel">Model · now {{ models?.defaultModel ?? st.s?.defaultModel }}</label>
        <div class="mt-1 flex gap-2">
          <input id="dmodel" v-model="model" class="input" list="dmodels" placeholder="fable, opus, sonnet…" :disabled="st.offline" />
          <datalist id="dmodels"><option v-for="m in models?.builtin ?? []" :key="m" :value="m" /></datalist>
          <button class="btn" :disabled="!model || st.offline" @click="set('model', model)">Set</button>
        </div>
      </div>
      <div>
        <label class="label" for="deffort">Effort · now {{ models?.defaultEffort ?? st.s?.defaultEffort }}</label>
        <select id="deffort" class="input mt-1" :value="models?.defaultEffort" :disabled="st.offline" @change="set('effort', ($event.target as HTMLSelectElement).value)">
          <option v-for="e in models?.effortLevels ?? []" :key="e" :value="e">{{ e }}</option>
        </select>
      </div>
      <div>
        <label class="label" for="dws">Workspace</label>
        <select id="dws" class="input mt-1" :value="st.s?.defaultWorkspace" :disabled="st.offline" @change="setWorkspace(($event.target as HTMLSelectElement).value)">
          <option v-for="w in st.s?.workspaces ?? []" :key="w.name" :value="w.name">{{ w.name }}</option>
        </select>
      </div>
    </div>
  </Section>
</template>
