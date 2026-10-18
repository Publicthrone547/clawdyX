<script setup lang="ts">
import { computed, ref } from "vue";
import Section from "./Section.vue";
import Modal from "../../components/Modal.vue";
import ListEditor from "../../components/ListEditor.vue";
import { System, errMsg, type D } from "../../api";
import { toast, fail } from "../../store";
import { st, fill, resetSection } from "./state";

const outputKB = computed({
  get: () => Math.round(st.draft.maxOutputBytes / 1024),
  set: (kb: number) => (st.draft.maxOutputBytes = Math.round(kb) * 1024),
});
// Folders on Windows start with a drive letter, elsewhere with "/".
const absolute = (v: string) => (/^([A-Za-z]:[\\/]|\/|\\\\)/.test(v) ? "" : "Give the full path of a folder.");

async function chooseFolder(key: "stateDir" | "logDir") {
  const p = await System.ChooseFolder(key === "stateDir" ? "Folder for clawdyX's state" : "Folder for clawdyX's logs").catch(fail);
  if (p) st.draft[key] = p;
}

const resetAllAsk = ref(false);
async function resetAll() {
  resetAllAsk.value = false;
  await resetSection("all");
}

async function exportSettings() {
  try {
    const where = await System.ExportSettings();
    if (where) toast(`Settings saved to ${where} (without the bot token).`, "ok");
  } catch (e) {
    fail(e);
  }
}
const preview = ref<D.ImportPreview | null>(null);
const allowOwner = ref(false);
const importErr = ref("");
async function chooseImport() {
  importErr.value = "";
  allowOwner.value = false;
  try {
    const p = await System.ChooseImport();
    if (p.file) preview.value = p;
  } catch (e) {
    toast("Not imported: " + errMsg(e), "danger");
  }
}
async function applyImport() {
  if (!preview.value) return;
  try {
    const r = await System.ApplyImport(preview.value.json, allowOwner.value);
    fill(r.settings);
    preview.value = null;
    toast("Settings imported. Restart the worker to apply.", "ok");
  } catch (e) {
    importErr.value = errMsg(e);
  }
}
</script>

<template>
  <Section title="Advanced" section="advanced">
    <details class="mt-2">
      <summary class="cursor-pointer text-sm">Show the advanced settings <span class="text-xs text-muted">· the defaults are right for almost everyone</span></summary>
      <div class="mt-3 grid grid-cols-2 gap-4">
        <div><label class="label" for="poll">Telegram wait per request (5–50 s)</label><input id="poll" v-model.number="st.draft.pollTimeoutSeconds" type="number" min="5" max="50" class="input mt-1" /></div>
        <div><label class="label" for="outkb">Largest reply kept per job (4–16384 KB)</label><input id="outkb" v-model.number="outputKB" type="number" min="4" max="16384" class="input mt-1" /></div>
        <div><label class="label" for="rss">Memory limit per Claude process (MB, at least 256)</label><input id="rss" v-model.number="st.draft.maxChildRssMb" type="number" min="256" class="input mt-1" /></div>
        <div><label class="label" for="bgw">Wait for background tasks after a reply (ms, 0–600000)</label><input id="bgw" v-model.number="st.draft.bgWaitCeilingMs" type="number" min="0" max="600000" step="1000" class="input mt-1" /></div>
        <div><label class="label" for="pud">Remember handled Telegram updates (days)</label><input id="pud" v-model.number="st.draft.processedUpdatesDays" type="number" min="1" class="input mt-1" /></div>
      </div>
      <div class="mt-4">
        <div class="flex items-center justify-between">
          <span class="label">Program folders for Claude (PATH, in order)</span>
          <button class="btn btn-ghost btn-sm" @click="st.draft.childPath = [...(st.s?.childPathDefault ?? [])]">Reset to default</button>
        </div>
        <ListEditor v-model="st.draft.childPath" class="mt-1.5" label="Program folders" placeholder="full path of a folder" mono ordered :check="absolute" />
      </div>
      <div class="mt-4 grid grid-cols-2 gap-4">
        <div>
          <label class="label" for="sdir">State folder</label>
          <div class="mt-1 flex gap-2"><input id="sdir" v-model="st.draft.stateDir" class="input mono" spellcheck="false" /><button class="btn" @click="chooseFolder('stateDir')">Choose…</button></div>
          <p class="mt-1 text-xs text-muted">Sessions, jobs and the audit log. Existing data is not moved.</p>
        </div>
        <div>
          <label class="label" for="ldir">Log folder</label>
          <div class="mt-1 flex gap-2"><input id="ldir" v-model="st.draft.logDir" class="input mono" spellcheck="false" /><button class="btn" @click="chooseFolder('logDir')">Choose…</button></div>
        </div>
      </div>
    </details>
    <div class="mt-4 flex flex-wrap gap-2 border-t border-line pt-3">
      <button class="btn" @click="exportSettings">Export settings…</button>
      <button class="btn" @click="chooseImport">Import settings…</button>
      <button class="btn btn-danger" @click="resetAllAsk = true">Reset all settings…</button>
    </div>
    <p class="mt-1 text-xs text-muted">The exported file holds every setting but never the bot token.</p>

    <Modal v-if="resetAllAsk" title="Reset all settings?" @close="resetAllAsk = false">
      <p>Every setting goes back to its default. The Telegram owner and allowed users, the bot token and the workspaces' names and folders stay; the workspaces get the default rules.</p>
      <template #actions>
        <button class="btn" @click="resetAllAsk = false">Cancel</button>
        <button class="btn btn-danger" @click="resetAll">Reset all</button>
      </template>
    </Modal>
    <Modal v-if="preview" :title="`Import ${preview.file}?`" wide @close="preview = null">
      <p v-if="!preview.result.changes?.length">The file holds the same settings as now; importing changes nothing.</p>
      <template v-else>
        <p>These settings change:</p>
        <ul class="mono mt-2 max-h-72 overflow-auto rounded-md border border-line bg-surface-2 px-3 py-2">
          <li v-for="c in preview.result.changes ?? []" :key="c">{{ c }}</li>
        </ul>
      </template>
      <label v-if="preview.result.ownerChanged" class="mt-3 flex items-start gap-2"><input v-model="allowOwner" type="checkbox" class="mt-0.5" /><span style="color: var(--danger)">The file is for another Telegram owner. Use it anyway.</span></label>
      <p v-if="importErr" class="mt-3 text-sm" style="color: var(--danger)">{{ importErr }}</p>
      <template #actions>
        <button class="btn" @click="preview = null">Cancel</button>
        <button class="btn btn-primary" :disabled="preview.result.ownerChanged && !allowOwner" @click="applyImport">Import</button>
      </template>
    </Modal>
  </Section>
</template>
