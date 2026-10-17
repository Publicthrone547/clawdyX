<script setup lang="ts">
import type { Tone } from "../util";
defineProps<{ tone: Tone; label: string }>();
const colour: Record<Tone, string> = {
  ok: "var(--ok)",
  warn: "var(--warn)",
  danger: "var(--danger)",
  muted: "var(--muted)",
  accent: "var(--accent)",
};
</script>

<template>
  <span
    class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium"
    :style="{ color: colour[tone], background: `color-mix(in srgb, ${colour[tone]} 14%, transparent)` }"
  >
    <span aria-hidden="true" class="inline-block h-1.5 w-1.5 rounded-full" :style="{ background: colour[tone] }"></span>
    {{ label }}
  </span>
</template>
