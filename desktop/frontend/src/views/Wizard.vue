<script setup lang="ts">
import { computed, ref } from "vue";
import SecretInput from "../components/SecretInput.vue";
import Badge from "../components/Badge.vue";
import { Setup, System, errMsg, type D, type Sender } from "../api";
import { store, refreshSetup, refreshService, toast } from "../store";

const steps = ["Welcome", "Telegram bot", "Who may use it", "Claude Code and folder", "Finish"];
const step = ref(0);
const s = computed(() => store.setup);

const token = ref("");
const botName = ref("");
const tokenErr = ref("");
const testing = ref(false);

const ownerId = ref("");
const senders = ref<Sender[]>([]);
const detectErr = ref("");
const detecting = ref(false);

const claude = ref<D.ClaudeCheck | null>(null);
const checkingClaude = ref(false);
const folder = ref<D.FolderCheck | null>(s.value?.suggestFolder ? { path: s.value.suggestFolder, ok: true } : null);
const folderText = ref(s.value?.suggestFolder ?? "");

const startService = ref(true);
const overwrite = ref(false);
const finishing = ref(false);
const finishErr = ref("");

const ownerOK = computed(() => /^[1-9][0-9]{4,15}$/.test(ownerId.value.trim()));

async function testToken() {
  testing.value = true;
  tokenErr.value = botName.value = "";
  try {
    botName.value = await Setup.TestToken(token.value.trim());
  } catch (e) {
    tokenErr.value = errMsg(e);
  } finally {
    testing.value = false;
  }
}
async function detect() {
  detecting.value = true;
  detectErr.value = "";
  try {
    senders.value = (await Setup.DetectOwner(token.value.trim())) ?? [];
    if (!senders.value.length) detectErr.value = "No private message to the bot was found yet. Send one in Telegram, then try again.";
    if (senders.value.length === 1) ownerId.value = String(senders.value[0].id);
  } catch (e) {
    detectErr.value = errMsg(e);
  } finally {
    detecting.value = false;
  }
}
async function checkClaude() {
  checkingClaude.value = true;
  try {
    claude.value = await Setup.CheckClaude(s.value?.claudeBinary ?? "");
  } finally {
    checkingClaude.value = false;
  }
}
async function chooseFolder() {
  try {
    const f = await Setup.ChooseFolder();
    if (f?.path) {
      folder.value = f;
      folderText.value = f.path;
    }
  } catch (e) {
    toast(errMsg(e), "danger");
  }
}
async function checkFolder() {
  folder.value = await Setup.CheckFolder(folderText.value);
}
async function finish() {
  finishing.value = true;
  finishErr.value = "";
  try {
    const r = await Setup.Finish({
      token: token.value.trim(),
      ownerId: Number(ownerId.value.trim()),
      workspace: folder.value?.path ?? "",
      browser: s.value?.browser ?? "",
      claudeBinary: claude.value?.binary ?? s.value?.claudeBinary ?? "",
      overwrite: overwrite.value,
      startService: startService.value,
    });
    token.value = "";
    toast(r.message, "ok");
    await refreshSetup();
    await refreshService();
  } catch (e) {
    finishErr.value = errMsg(e);
  } finally {
    finishing.value = false;
  }
}
const canNext = computed(() => {
  switch (step.value) {
    case 1:
      return !!botName.value;
    case 2:
      return ownerOK.value;
    case 3:
      return !!folder.value?.ok && !!claude.value && !claude.value.error;
    default:
      return true;
  }
});
function next() {
  if (step.value === 3 && !claude.value) checkClaude();
  if (canNext.value && step.value < steps.length - 1) {
    step.value++;
    if (step.value === 3 && !claude.value) checkClaude();
  }
}
</script>

<template>
  <div class="fixed inset-0 z-30 flex items-center justify-center bg-bg/95 p-8">
    <div class="card flex h-[34rem] w-full max-w-3xl overflow-hidden shadow-2xl" role="dialog" aria-modal="true" aria-label="Set up clawdyX">
      <ol class="w-52 shrink-0 border-r border-line bg-surface-2 px-4 py-6">
        <li v-for="(name, i) in steps" :key="name" class="flex items-center gap-2 py-1.5" :class="i === step ? 'font-semibold' : 'text-muted'" :aria-current="i === step ? 'step' : undefined">
          <span class="inline-flex h-5 w-5 items-center justify-center rounded-full text-[10px]" :style="i <= step ? 'background: var(--accent); color: var(--accent-text)' : 'border: 1px solid var(--border)'">{{ i + 1 }}</span>
          {{ name }}
        </li>
      </ol>
      <div class="flex flex-1 flex-col">
        <div class="flex-1 overflow-auto px-7 py-6">
          <template v-if="step === 0">
            <h2 class="text-lg font-semibold">Set up clawdyX</h2>
            <p class="mt-3">clawdyX lets you talk to the Claude Code installed on this Mac from Telegram. A background worker runs the bot; this app controls it.</p>
            <ul class="mt-4 list-disc space-y-1.5 pl-5 text-muted">
              <li>It uses your existing Claude Code login. It never asks for an API key and never logs in for you.</li>
              <li>It never bypasses Claude Code's permission system. New sessions start read-only.</li>
              <li>Only the Telegram accounts you list can use the bot; everyone else is ignored silently.</li>
              <li>Screens are captured only when you ask. Nothing is captured at startup or on a timer.</li>
            </ul>
            <p v-if="s?.configExists" class="mt-4 text-warn">A configuration already exists at <span class="mono">{{ s.configPath }}</span>{{ s.configError ? ` but it is invalid: ${s.configError}` : "" }}. Finishing this wizard replaces it.</p>
          </template>

          <template v-else-if="step === 1">
            <h2 class="text-lg font-semibold">Telegram bot</h2>
            <p class="mt-2 text-muted">In Telegram, open <a href="#" class="text-accent underline" @click.prevent="System.Open('botfather')">@BotFather</a>, send <span class="mono">/newbot</span>, and copy the token it gives you. Do not share it with anyone.</p>
            <label for="tok" class="label mt-5 block">Bot token</label>
            <div class="mt-1.5"><SecretInput id="tok" v-model="token" placeholder="123456789:AA…" /></div>
            <div class="mt-3 flex items-center gap-3">
              <button class="btn" :disabled="!token || testing" @click="testToken">{{ testing ? "Testing…" : "Test connection" }}</button>
              <Badge v-if="botName" tone="ok" :label="'Connected as @' + botName" />
            </div>
            <p v-if="tokenErr" class="mt-2 text-danger" role="alert">{{ tokenErr }}</p>
            <p class="mt-4 text-xs text-muted">The token is stored in the macOS Keychain when you finish. It is never shown again or written to a settings file.</p>
          </template>

          <template v-else-if="step === 2">
            <h2 class="text-lg font-semibold">Who may use the bot</h2>
            <p class="mt-2 text-muted">Enter your numeric Telegram user ID. You become the bot's owner; you can add other people later. Usernames are not used for access.</p>
            <label for="owner" class="label mt-5 block">Your Telegram user ID</label>
            <input id="owner" v-model="ownerId" class="input mono mt-1.5 w-60" inputmode="numeric" placeholder="123456789" />
            <p v-if="ownerId && !ownerOK" class="mt-1 text-danger">An ID is a positive number, usually 9–10 digits.</p>
            <div class="mt-5 card px-4 py-3">
              <div class="font-medium">Find it automatically</div>
              <p class="mt-1 text-muted">Send any message to <b>@{{ botName }}</b> in Telegram, then press Detect. Pending messages are cleared so they are not answered later.</p>
              <button class="btn mt-2" :disabled="detecting" @click="detect">{{ detecting ? "Looking…" : "Detect" }}</button>
              <p v-if="detectErr" class="mt-2 text-warn">{{ detectErr }}</p>
              <ul v-if="senders.length > 1" class="mt-2 space-y-1">
                <li v-for="x in senders" :key="x.id">
                  <label class="flex items-center gap-2"><input v-model="ownerId" type="radio" :value="String(x.id)" /> <span class="mono">{{ x.id }}</span> {{ x.firstName }} <span v-if="x.username" class="text-muted">@{{ x.username }}</span></label>
                </li>
              </ul>
              <p v-else-if="senders.length === 1" class="mt-2">Found <span class="mono">{{ senders[0].id }}</span> ({{ senders[0].firstName }}).</p>
            </div>
          </template>

          <template v-else-if="step === 3">
            <h2 class="text-lg font-semibold">Claude Code and your project folder</h2>
            <div class="mt-4 card px-4 py-3">
              <div class="flex items-center gap-2">
                <span class="font-medium">Claude Code</span>
                <Badge v-if="checkingClaude" tone="warn" label="Checking (about 30 s)…" />
                <Badge v-else-if="claude && !claude.error" tone="ok" :label="`${claude.version} · ${claude.subscription || claude.method}`" />
                <Badge v-else-if="claude?.error" tone="danger" label="Problem" />
              </div>
              <p v-if="claude?.error" class="mt-1 text-danger" role="alert">{{ claude.error }}</p>
              <p v-else class="mt-1 text-xs text-muted mono">{{ claude?.binary || s?.claudeBinary || "not found" }}</p>
              <button class="btn btn-sm mt-2" :disabled="checkingClaude" @click="checkClaude">Check again</button>
            </div>
            <label for="ws" class="label mt-5 block">Project folder Claude may work in</label>
            <div class="mt-1.5 flex gap-2">
              <input id="ws" v-model="folderText" class="input mono" placeholder="/Users/you/projects/app" @change="checkFolder" />
              <button class="btn" @click="chooseFolder">Choose…</button>
            </div>
            <p v-if="folder?.error" class="mt-1 text-danger">{{ folder.error }}</p>
            <p v-else-if="folder?.warning" class="mt-1 text-warn">{{ folder.warning }}</p>
            <p class="mt-3 text-xs text-muted">Claude can only read and edit inside configured folders. Sessions start in the read-only <b>observe</b> tier.</p>
          </template>

          <template v-else>
            <h2 class="text-lg font-semibold">Finish</h2>
            <dl class="mt-4 grid grid-cols-[10rem_1fr] gap-y-2">
              <dt class="text-muted">Bot</dt><dd>@{{ botName }} · token ends …{{ token.slice(-4) }}</dd>
              <dt class="text-muted">Owner</dt><dd class="mono">{{ ownerId }}</dd>
              <dt class="text-muted">Claude Code</dt><dd>{{ claude?.version }} · {{ claude?.subscription || claude?.method }}</dd>
              <dt class="text-muted">Folder</dt><dd class="mono">{{ folder?.path }}</dd>
            </dl>
            <label class="mt-5 flex items-center gap-2"><input v-model="startService" type="checkbox" /> Start the background worker now and at every login</label>
            <label v-if="s?.configExists" class="mt-2 flex items-center gap-2 text-warn"><input v-model="overwrite" type="checkbox" /> Replace the existing configuration</label>
            <p v-if="finishErr" class="mt-3 text-danger" role="alert">{{ finishErr }}</p>
          </template>
        </div>
        <div class="flex items-center justify-between border-t border-line px-7 py-3">
          <button class="btn" :disabled="step === 0" @click="step--">Back</button>
          <button v-if="step < steps.length - 1" class="btn btn-primary" :disabled="!canNext" @click="next">Continue</button>
          <button v-else class="btn btn-primary" :disabled="finishing || (s?.configExists && !overwrite)" @click="finish">{{ finishing ? "Saving…" : "Finish" }}</button>
        </div>
      </div>
    </div>
  </div>
</template>
