<script setup lang="ts">
defineProps<{ text: string }>();
</script>

<template>
  <div class="rounded-lg border border-dashed border-line px-4 py-8 text-center text-muted">
    {{ text }}
    <div v-if="$slots.default" class="mt-3"><slot /></div>
  </div>
</template>
