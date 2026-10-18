<script setup lang="ts">
import Section from "./Section.vue";
import { System } from "../../api";
import { fail } from "../../store";
import { words } from "../../platform";
import { st } from "./state";
</script>

<template>
  <Section title="Logs and retention" section="logs">
    <div class="mt-3 grid grid-cols-4 gap-4">
      <div><label class="label" for="lvl">Log level</label><select id="lvl" v-model="st.draft.logLevel" class="input mt-1"><option>debug</option><option>info</option><option>warn</option><option>error</option></select></div>
      <div><label class="label" for="jd">Keep jobs (days)</label><input id="jd" v-model.number="st.draft.jobsDays" type="number" min="1" class="input mt-1" /></div>
      <div><label class="label" for="ed">Keep job activity (days)</label><input id="ed" v-model.number="st.draft.jobEventsDays" type="number" min="1" class="input mt-1" /></div>
      <div><label class="label" for="ad">Keep audit log (days)</label><input id="ad" v-model.number="st.draft.auditDays" type="number" min="1" class="input mt-1" /></div>
    </div>
    <p class="mt-2 text-xs text-muted">Logs rotate at 20 MB × 5 in <span class="mono">{{ st.s?.logDir }}</span>. <a href="#" class="underline" @click.prevent="System.RevealLogs().catch(fail)">Show in {{ words.fileManager }}</a></p>
  </Section>
</template>
