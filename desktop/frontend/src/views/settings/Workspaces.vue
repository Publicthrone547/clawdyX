<script setup lang="ts">
import { ref } from "vue";
import Section from "./Section.vue";
import Badge from "../../components/Badge.vue";
import Modal from "../../components/Modal.vue";
import Rules from "./Rules.vue";
import { Setup, type WorkspaceInfo } from "../../api";
import { toast, fail } from "../../store";
import { words } from "../../platform";
import { st, patchNow } from "./state";

const rulesOf = ref<WorkspaceInfo | null>(null);
const renaming = ref<WorkspaceInfo | null>(null);
const newName = ref("");

async function add() {
  try {
    const fc = await Setup.ChooseFolder();
    if (!fc?.path) return;
    if (!fc.ok) return toast(fc.error ?? "That folder cannot be used", "danger");
    if (fc.warning) toast(fc.warning);
    await patchNow({ addWorkspace: { name: "", path: fc.path, defaultMode: "observe", allowedCommands: [], deniedCommands: [], iKnowWhatIAmDoing: false, isDefault: false } }, "Workspace added. Restart the worker to use it.");
  } catch (e) {
    fail(e);
  }
}
function startRename(w: WorkspaceInfo) {
  renaming.value = w;
  newName.value = w.name;
}
async function rename() {
  const w = renaming.value;
  if (!w) return;
  if (await patchNow({ renameWorkspace: { from: w.name, to: newName.value.trim() } }, "Workspace renamed. Restart the worker to apply.")) renaming.value = null;
}
</script>

<template>
  <Section title="Workspaces" section="workspaces" :hint="`Claude can only work inside these folders on ${words.thisComputer}. The tier is the one new sessions start in. Reset gives every workspace the default tier and rules; names and folders stay.`">
    <div class="mt-2 flex justify-end"><button class="btn" @click="add">Add folder…</button></div>
    <table class="tbl mt-2">
      <thead><tr><th>Name</th><th>Folder</th><th>Starting tier</th><th>Rules</th><th></th></tr></thead>
      <tbody>
        <tr v-for="w in st.s?.workspaces ?? []" :key="w.name">
          <td><b>{{ w.name }}</b> <Badge v-if="w.isDefault" tone="accent" label="default" /> <Badge v-if="w.iKnowWhatIAmDoing" tone="danger" label="minimum rules off" /></td>
          <td class="mono">{{ w.path }}</td>
          <td>
            <select class="input py-1" :value="w.defaultMode" :aria-label="`Starting tier of ${w.name}`" @change="patchNow({ workspaceModes: { [w.name]: ($event.target as HTMLSelectElement).value } })">
              <option value="observe">observe</option><option value="assisted">assisted</option>
            </select>
          </td>
          <td class="text-xs text-muted">{{ w.allowedCommands?.length ?? 0 }} allowed · {{ w.deniedCommands?.length ?? 0 }} denied</td>
          <td class="flex justify-end gap-1.5">
            <button class="btn btn-sm" @click="rulesOf = w">Rules…</button>
            <button class="btn btn-sm" :disabled="st.offline" :title="st.offline ? 'Needs the worker (it knows which sessions use the workspace)' : ''" @click="startRename(w)">Rename…</button>
            <button class="btn btn-danger btn-sm" :disabled="(st.s?.workspaces?.length ?? 0) < 2 || st.offline" @click="patchNow({ removeWorkspace: w.name }, 'Workspace removed. Restart the worker to apply.')">Remove</button>
          </td>
        </tr>
      </tbody>
    </table>
    <Rules v-if="rulesOf" :ws="rulesOf" @close="rulesOf = null" />
    <Modal v-if="renaming" :title="`Rename ${renaming.name}`" @close="renaming = null">
      <label class="label" for="wsname">New name</label>
      <input id="wsname" v-model="newName" class="input mt-1 mono" autofocus @keydown.enter="rename" />
      <p class="mt-1 text-xs text-muted">Lowercase letters, digits, - and _. Only possible while no session uses the workspace.</p>
      <template #actions>
        <button class="btn" @click="renaming = null">Cancel</button>
        <button class="btn btn-primary" :disabled="!newName.trim() || newName.trim() === renaming.name" @click="rename">Rename</button>
      </template>
    </Modal>
  </Section>
</template>
