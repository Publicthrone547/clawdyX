<script setup lang="ts">
import { ref } from "vue";
const model = defineModel<string>({ default: "" });
defineProps<{ id: string; placeholder?: string }>();
const show = ref(false);
</script>

<template>
  <div class="flex gap-2">
    <input
      :id="id"
      v-model="model"
      :type="show ? 'text' : 'password'"
      class="input mono"
      autocomplete="off"
      autocapitalize="off"
      spellcheck="false"
      :placeholder="placeholder"
    />
    <button type="button" class="btn btn-sm" :aria-pressed="show" :aria-label="show ? 'Hide token' : 'Show token'" @click="show = !show">
      {{ show ? "Hide" : "Show" }}
    </button>
  </div>
</template>
