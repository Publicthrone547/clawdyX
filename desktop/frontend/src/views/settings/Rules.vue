<script setup lang="ts">
import { ref } from "vue";
import Modal from "../../components/Modal.vue";
import ListEditor from "../../components/ListEditor.vue";
import { Worker, System, errMsg, type WorkspaceInfo } from "../../api";
import { toast } from "../../store";
import { words } from "../../platform";
import { st, fill } from "./state";
import { checkRule, presets, minimumDenied } from "./rules";

const props = defineProps<{ ws: WorkspaceInfo }>();
const emit = defineEmits<{ close: [] }>();
const allowed = ref([...(props.ws.allowedCommands ?? [])]);
const denied = ref([...(props.ws.deniedCommands ?? [])]);
const optOut = ref(props.ws.iKnowWhatIAmDoing);
const err = ref("");
const saving = ref(false);

function addPreset(i: number) {
  for (const r of presets[i].allowed) if (!allowed.value.includes(r)) allowed.value.push(r);
}
const missingMinimum = () => minimumDenied.filter((r) => !denied.value.includes(r));

async function save() {
  err.value = "";
  saving.value = true;
  const p = { workspaceRules: { [props.ws.name]: { allowed: allowed.value, denied: denied.value, iKnowWhatIAmDoing: optOut.value } } };
  try {
    fill(st.offline ? await System.SetSettingsOffline(p) : await Worker.SetSettings(p));
    toast("Rules saved. Restart the worker to apply.", "ok");
    emit("close");
  } catch (e) {
    // The worker's validation message, for example
    // `workspace "x" denied_commands must include Bash(rm *)`.
    err.value = errMsg(e);
  } finally {
    saving.value = false;
  }
}
</script>

<template>
  <Modal :title="`Command rules of ${ws.name}`" wide @close="emit('close')">
    <p class="text-xs text-muted">Rules are Claude Code permission rules such as <span class="mono">Bash(npm test *)</span>. Denied rules win over allowed ones.</p>
    <div class="mt-4">
      <span class="label">Allowed without asking (assisted tier)</span>
      <ListEditor v-model="allowed" class="mt-1.5" label="Allowed commands" placeholder="Bash(npm test *)" mono :check="(v) => checkRule(v)" />
      <div class="mt-2 flex flex-wrap items-center gap-2 text-xs">
        <span class="text-muted">Add a preset:</span>
        <button v-for="(p, i) in presets" :key="p.name" class="btn btn-sm" @click="addPreset(i)">{{ p.name }}</button>
      </div>
    </div>
    <div class="mt-4">
      <span class="label">Denied in this workspace</span>
      <ListEditor v-model="denied" class="mt-1.5" label="Denied commands" placeholder="Bash(make deploy *)" mono :check="(v) => checkRule(v)" />
      <p v-if="missingMinimum().length && !optOut" class="mt-1 text-xs" style="color: var(--warn)">Without the opt-out below, the list must keep {{ missingMinimum().join(", ") }}.</p>
    </div>
    <details class="mt-4">
      <summary class="cursor-pointer text-sm">Always denied on {{ words.osName }} ({{ st.s?.builtinDenied?.length ?? 0 }} rules, built in)</summary>
      <ListEditor :model-value="st.s?.builtinDenied ?? []" class="mt-2" label="Always denied" mono readonly />
    </details>
    <label class="mt-4 flex items-start gap-2"><input v-model="optOut" type="checkbox" class="mt-0.5" /><span><b style="color: var(--danger)">I know what I am doing</b><span class="block text-xs text-muted">Removes the minimum deny rules (rm, sudo, git push). Only for people who understand Claude Code permissions. The built-in rules above stay.</span></span></label>
    <p v-if="err" class="mt-3 text-sm" style="color: var(--danger)">{{ err }}</p>
    <template #actions>
      <button class="btn" @click="emit('close')">Cancel</button>
      <button class="btn btn-primary" :disabled="saving" @click="save">Save rules</button>
    </template>
  </Modal>
</template>
