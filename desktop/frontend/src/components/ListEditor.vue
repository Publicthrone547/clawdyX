<script setup lang="ts">
import { ref } from "vue";

// ListEditor edits a list of strings in place: remove an entry, add one
// (checked by `check`, which returns an error text or ""), keep the order.
const props = defineProps<{ modelValue: string[]; placeholder?: string; label: string; check?: (v: string) => string; mono?: boolean; readonly?: boolean; ordered?: boolean }>();
const emit = defineEmits<{ "update:modelValue": [string[]] }>();
const next = ref("");
const err = ref("");
function add() {
  const v = next.value.trim();
  if (!v) return;
  err.value = props.check?.(v) ?? "";
  if (err.value) return;
  if (props.modelValue.includes(v)) {
    err.value = "Already in the list.";
    return;
  }
  emit("update:modelValue", [...props.modelValue, v]);
  next.value = "";
}
function remove(i: number) {
  const l = [...props.modelValue];
  l.splice(i, 1);
  emit("update:modelValue", l);
}
function move(i: number, d: number) {
  const l = [...props.modelValue];
  const j = i + d;
  if (j < 0 || j >= l.length) return;
  [l[i], l[j]] = [l[j], l[i]];
  emit("update:modelValue", l);
}
</script>

<template>
  <div>
    <ul class="flex flex-wrap gap-1.5" :aria-label="label">
      <li v-for="(v, i) in modelValue" :key="v" class="flex items-center gap-1 rounded-md border border-line bg-surface-2 py-0.5 pr-1 pl-2" :class="mono ? 'mono' : 'text-xs'">
        <span>{{ v }}</span>
        <template v-if="!readonly">
          <button v-if="ordered" class="btn btn-ghost btn-sm px-1" :aria-label="`Move ${v} earlier`" :disabled="i === 0" @click="move(i, -1)">↑</button>
          <button class="btn btn-ghost btn-sm px-1" :aria-label="`Remove ${v}`" @click="remove(i)">✕</button>
        </template>
      </li>
      <li v-if="!modelValue.length" class="text-xs text-muted">none</li>
    </ul>
    <div v-if="!readonly" class="mt-2 flex gap-2">
      <input v-model="next" class="input" :class="mono ? 'mono' : ''" :placeholder="placeholder" :aria-label="`Add to ${label}`" @keydown.enter.prevent="add" />
      <button class="btn" :disabled="!next.trim()" @click="add">Add</button>
    </div>
    <p v-if="err" class="mt-1 text-xs" style="color: var(--danger)">{{ err }}</p>
  </div>
</template>
