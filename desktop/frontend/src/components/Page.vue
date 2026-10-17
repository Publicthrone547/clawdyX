<script setup lang="ts">
defineProps<{ title: string; subtitle?: string }>();
</script>

<template>
  <section class="flex h-full flex-col">
    <header class="drag flex items-end justify-between gap-4 px-7 pt-6 pb-4">
      <div>
        <h1 class="text-xl font-semibold tracking-tight">{{ title }}</h1>
        <p v-if="subtitle" class="mt-0.5 text-muted">{{ subtitle }}</p>
      </div>
      <div class="nodrag flex items-center gap-2"><slot name="actions" /></div>
    </header>
    <div class="flex-1 overflow-auto px-7 pb-8"><slot /></div>
  </section>
</template>
