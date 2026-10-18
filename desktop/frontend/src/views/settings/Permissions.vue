<script setup lang="ts">
import Section from "./Section.vue";
import ListEditor from "../../components/ListEditor.vue";
import { words } from "../../platform";
import { st } from "./state";
import { checkRule } from "./rules";
</script>

<template>
  <Section title="Permissions" section="permissions" hint="Denied in every workspace and every tier, in addition to each workspace's own rules.">
    <div class="mt-3">
      <span class="label">Extra denied tools and commands</span>
      <ListEditor v-model="st.draft.extraDisallowedTools" class="mt-1.5" label="Extra denied tools and commands" placeholder="Bash(make deploy *) or a tool name such as NotebookEdit" mono :check="(v) => checkRule(v, true)" />
    </div>
    <details class="mt-4">
      <summary class="cursor-pointer text-sm">Always denied on {{ words.osName }} ({{ st.s?.builtinDenied?.length ?? 0 }} rules, built in)</summary>
      <p class="mt-1 text-xs text-muted">clawdyX adds these to every job; they cannot be switched off here.</p>
      <ListEditor :model-value="st.s?.builtinDenied ?? []" class="mt-2" label="Always denied" mono readonly />
    </details>
  </Section>
</template>
