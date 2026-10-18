<script setup lang="ts">
import { computed } from "vue";
import Section from "./Section.vue";
import { st } from "./state";

// The budget: 0 is off, otherwise at least $2 (Claude Code's minimum that
// still lets a session start).
const budgetOn = computed({
  get: () => st.draft.maxBudgetUsd > 0,
  set: (on: boolean) => (st.draft.maxBudgetUsd = on ? Math.max(st.draft.maxBudgetUsd, 5) : 0),
});
</script>

<template>
  <Section title="Limits" section="limits" hint="How much Claude may do per job and at once.">
    <div class="mt-3 grid grid-cols-3 gap-4">
      <div><label class="label" for="conc">Jobs at once (1–4)</label><input id="conc" v-model.number="st.draft.maxConcurrentJobs" type="number" min="1" max="4" class="input mt-1" /></div>
      <div><label class="label" for="tmo">Job timeout (minutes, 1–120)</label><input id="tmo" v-model.number="st.draft.jobTimeoutMinutes" type="number" min="1" max="120" class="input mt-1" /></div>
      <div><label class="label" for="turns">Max turns per job (1–500)</label><input id="turns" v-model.number="st.draft.maxTurns" type="number" min="1" max="500" class="input mt-1" /></div>
      <div><label class="label" for="queue">Waiting prompts per session (1–50)</label><input id="queue" v-model.number="st.draft.maxQueuePerSession" type="number" min="1" max="50" class="input mt-1" /></div>
      <div class="col-span-2">
        <span class="label">Cost limit per job</span>
        <div class="mt-1 flex items-center gap-3">
          <label class="flex items-center gap-2"><input v-model="budgetOn" type="checkbox" /> Stop a job at</label>
          <input v-model.number="st.draft.maxBudgetUsd" type="number" min="2" step="1" class="input w-28" :disabled="!budgetOn" aria-label="Cost limit in US dollars" />
          <span class="text-muted">USD</span>
        </div>
        <p class="mt-1 text-xs text-muted">Claude Code's own estimate at list prices (also with a subscription). At least $2; off by default.</p>
      </div>
    </div>
  </Section>
</template>
