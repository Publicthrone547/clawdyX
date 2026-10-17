<script setup lang="ts">
import { nextTick, onMounted, onBeforeUnmount, ref } from "vue";
const props = defineProps<{ title: string; wide?: boolean }>();
const emit = defineEmits<{ close: [] }>();
const box = ref<HTMLElement | null>(null);
let prev: Element | null = null;
function key(e: KeyboardEvent) {
  if (e.key === "Escape") {
    e.stopPropagation();
    emit("close");
  }
  if (e.key === "Tab" && box.value) {
    // keep focus inside the dialog
    const f = box.value.querySelectorAll<HTMLElement>("button, input, select, textarea, [tabindex]:not([tabindex='-1'])");
    if (!f.length) return;
    const first = f[0], last = f[f.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      last.focus();
      e.preventDefault();
    } else if (!e.shiftKey && document.activeElement === last) {
      first.focus();
      e.preventDefault();
    }
  }
}
onMounted(async () => {
  prev = document.activeElement;
  await nextTick();
  const f = box.value?.querySelector<HTMLElement>("[autofocus], input, textarea, select, button:not(.btn-danger)");
  f?.focus();
});
onBeforeUnmount(() => (prev as HTMLElement | null)?.focus?.());
void props;
</script>

<template>
  <div class="fixed inset-0 z-40 flex items-center justify-center bg-black/40 p-6" @keydown="key" @mousedown.self="emit('close')">
    <div
      ref="box"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      class="card flex max-h-full w-full flex-col shadow-2xl"
      :class="wide ? 'max-w-3xl' : 'max-w-lg'"
    >
      <div class="flex items-center justify-between border-b border-line px-5 py-3">
        <h2 class="text-sm font-semibold">{{ title }}</h2>
        <button class="btn btn-ghost btn-sm" aria-label="Close" @click="emit('close')">✕</button>
      </div>
      <div class="overflow-auto px-5 py-4"><slot /></div>
      <div v-if="$slots.actions" class="flex justify-end gap-2 border-t border-line px-5 py-3"><slot name="actions" /></div>
    </div>
  </div>
</template>
