<script setup lang="ts">
import { ref } from "vue";
import Section from "./Section.vue";
import { System, Setup } from "../../api";
import { toast, fail } from "../../store";
import { isWindows } from "../../platform";
import { st } from "./state";

const checking = ref(false);
const result = ref("");
async function check() {
  checking.value = true;
  result.value = "";
  try {
    const c = await Setup.CheckClaude(st.draft.claudeBinary);
    result.value = c.error ? c.error : `Claude Code ${c.version} · logged in (${c.method}${c.subscription ? ", " + c.subscription : ""})`;
  } catch (e) {
    fail(e);
  } finally {
    checking.value = false;
  }
}
async function detect() {
  const p = await System.DetectClaude().catch(() => "");
  if (p) st.draft.claudeBinary = p;
  else toast("Claude Code was not found on this computer's usual paths.", "danger");
}
async function browse() {
  const p = await System.ChooseFile("Choose the claude program").catch(fail);
  if (p) st.draft.claudeBinary = p;
}
async function detectGitBash() {
  const p = await System.DetectGitBash().catch(() => "");
  if (p) st.draft.gitBashPath = p;
  else toast("Git for Windows was not found. Install it, or choose its bash.exe.", "danger");
}
async function browseGitBash() {
  const p = await System.ChooseFile("Choose bash.exe of Git for Windows").catch(fail);
  if (p) st.draft.gitBashPath = p;
}
const sources = [
  { id: "user", label: "Your own Claude Code settings" },
  { id: "project", label: "Settings checked into the project" },
  { id: "local", label: "Local settings of the project" },
];
function toggleSource(id: string, on: boolean) {
  const l = st.draft.settingSources.filter((x) => x !== id);
  if (on) l.push(id);
  st.draft.settingSources = sources.map((s) => s.id).filter((x) => l.includes(x));
}
</script>

<template>
  <Section title="Claude Code" section="claude-code" hint="Where the claude program is and what it loads for jobs. Changes apply after the worker restarts.">
    <div class="mt-3">
      <label class="label" for="cbin">Program</label>
      <div class="mt-1 flex gap-2">
        <input id="cbin" v-model="st.draft.claudeBinary" class="input mono" spellcheck="false" />
        <button class="btn" @click="detect">Detect</button>
        <button class="btn" @click="browse">Browse…</button>
        <button class="btn" :disabled="checking" @click="check">{{ checking ? "Checking…" : "Check" }}</button>
      </div>
      <p v-if="result" class="mt-1 text-xs text-muted">{{ result }}</p>
    </div>
    <div class="mt-4">
      <span class="label">Settings Claude Code loads for jobs</span>
      <label v-for="s in sources" :key="s.id" class="mt-1.5 flex items-center gap-2">
        <input type="checkbox" :checked="st.draft.settingSources.includes(s.id)" @change="toggleSource(s.id, ($event.target as HTMLInputElement).checked)" />
        {{ s.label }} <span class="mono text-muted">({{ s.id }})</span>
      </label>
      <label class="mt-3 flex items-start gap-2"><input v-model="st.draft.strictMcpConfig" type="checkbox" class="mt-0.5" /><span><b>Only clawdyX's MCP servers</b><span class="block text-xs text-muted">Jobs ignore the MCP servers of your Claude Code settings (recommended).</span></span></label>
    </div>
    <div v-if="isWindows" class="mt-4">
      <label class="label" for="gbash">Git Bash (Claude Code's shell)</label>
      <div class="mt-1 flex gap-2">
        <input id="gbash" v-model="st.draft.gitBashPath" class="input mono" spellcheck="false" placeholder="found automatically" />
        <button class="btn" @click="detectGitBash">Detect</button>
        <button class="btn" @click="browseGitBash">Browse…</button>
      </div>
      <p class="mt-1 text-xs text-muted">Claude Code runs shell commands through Git for Windows. Leave empty to let it find bash.exe itself.</p>
    </div>
  </Section>
</template>
