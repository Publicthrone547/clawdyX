<script setup lang="ts">
import { ref } from "vue";
import Modal from "../../components/Modal.vue";
import { resetSection } from "./state";

const props = defineProps<{ title: string; hint?: string; section?: string; id?: string }>();
const ask = ref(false);
async function reset() {
  ask.value = false;
  if (props.section) await resetSection(props.section);
}
</script>

<template>
  <section class="card mt-4 px-5 py-4" :aria-labelledby="id ? id + '-h' : undefined">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h2 :id="id ? id + '-h' : undefined" class="font-semibold">{{ title }}</h2>
        <p v-if="hint" class="text-xs text-muted">{{ hint }}</p>
      </div>
      <button v-if="section" class="btn btn-ghost btn-sm text-muted" @click="ask = true">Reset section</button>
    </div>
    <slot />
    <Modal v-if="ask" :title="`Reset ${title}?`" @close="ask = false">
      <p>The settings of this section go back to their defaults. Other sections stay as they are.</p>
      <template #actions>
        <button class="btn" @click="ask = false">Cancel</button>
        <button class="btn btn-danger" @click="reset">Reset</button>
      </template>
    </Modal>
  </section>
</template>
