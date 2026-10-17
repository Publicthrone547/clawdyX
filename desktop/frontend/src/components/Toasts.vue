<script setup lang="ts">
import { store } from "../store";
</script>

<template>
  <div class="pointer-events-none fixed right-4 bottom-4 z-50 flex w-96 flex-col gap-2" aria-live="polite" role="status">
    <div
      v-for="t in store.toasts"
      :key="t.id"
      class="card pointer-events-auto px-4 py-3 text-sm shadow-lg"
      :style="t.tone === 'danger' ? 'border-color: var(--danger)' : t.tone === 'ok' ? 'border-color: var(--ok)' : ''"
    >
      <span v-if="t.tone === 'danger'" class="font-semibold text-danger">Error: </span>{{ t.text }}
    </div>
  </div>
</template>
