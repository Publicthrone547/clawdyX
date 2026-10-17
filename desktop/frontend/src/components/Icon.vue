<script setup lang="ts">
// A small inline icon set (no icon font, no remote assets).
defineProps<{ name: string; size?: number }>();
const paths: Record<string, string> = {
  overview: "M3 13h8V3H3zm0 8h8v-6H3zm10 0h8V11h-8zm0-18v6h8V3z",
  telegram: "M21.5 4.5 2.8 11.7c-.9.4-.9 1.6.1 1.9l4.6 1.4 1.8 5.5c.3.8 1.3 1 1.9.4l2.6-2.5 4.7 3.4c.7.5 1.7.1 1.9-.7l3-15c.2-1-.8-1.8-1.9-1.6zM9.7 14.6l-.4 3.9-1.4-4.3 9.6-6.9z",
  sessions: "M4 5h16v3H4zm0 5.5h16v3H4zM4 16h10v3H4z",
  jobs: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 7v5l3 2",
  monitors: "M3 4h18v12H3zm2 2v8h14V6zM8 19h8v2H8z",
  settings: "M19.4 13a7.5 7.5 0 0 0 0-2l2.1-1.6-2-3.5-2.5 1a7.3 7.3 0 0 0-1.7-1L15 3h-4l-.4 2.6c-.6.3-1.2.6-1.7 1l-2.5-1-2 3.5L6.6 11a7.5 7.5 0 0 0 0 2l-2.1 1.6 2 3.5 2.5-1c.5.4 1.1.7 1.7 1L11 21h4l.4-2.6c.6-.3 1.2-.6 1.7-1l2.5 1 2-3.5zM13 15.5a3.5 3.5 0 1 1 0-7 3.5 3.5 0 0 1 0 7z",
  diagnostics: "M3 12h4l2-6 4 12 2-6h6",
  refresh: "M17.6 6.4A8 8 0 1 0 20 12h-2a6 6 0 1 1-1.8-4.2L13 11h7V4z",
  play: "M8 5v14l11-7z",
  stop: "M6 6h12v12H6z",
  pause: "M6 5h4v14H6zm8 0h4v14h-4z",
  plus: "M11 5h2v14h-2zM5 11h14v2H5z",
  close: "M6.4 5 12 10.6 17.6 5 19 6.4 13.4 12l5.6 5.6-1.4 1.4-5.6-5.6L6.4 19 5 17.6l5.6-5.6L5 6.4z",
  camera: "M9 4 7.2 6H4v13h16V6h-3.2L15 4zm3 4a4.5 4.5 0 1 1 0 9 4.5 4.5 0 0 1 0-9z",
  check: "M9 16.2 4.8 12l-1.4 1.4L9 19 21 7l-1.4-1.4z",
  warn: "M1 21h22L12 2zm12-3h-2v-2h2zm0-4h-2v-4h2z",
  eye: "M12 5C7 5 2.7 8.1 1 12.5 2.7 16.9 7 20 12 20s9.3-3.1 11-7.5C21.3 8.1 17 5 12 5zm0 12.5a5 5 0 1 1 0-10 5 5 0 0 1 0 10z",
  lock: "M7 10V7a5 5 0 0 1 10 0v3h2v11H5V10zm2 0h6V7a3 3 0 0 0-6 0z",
};
</script>

<template>
  <svg :width="size ?? 18" :height="size ?? 18" viewBox="0 0 24 24" aria-hidden="true" fill="currentColor">
    <path v-if="name !== 'diagnostics' && name !== 'jobs'" :d="paths[name]" />
    <path v-else :d="paths[name]" fill="none" stroke="currentColor" stroke-width="2" stroke-linejoin="round" stroke-linecap="round" />
  </svg>
</template>
