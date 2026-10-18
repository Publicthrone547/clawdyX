<script setup lang="ts">
import { computed, ref, watch, onBeforeUnmount } from "vue";
import Page from "../components/Page.vue";
import Badge from "../components/Badge.vue";
import Modal from "../components/Modal.vue";
import SecretInput from "../components/SecretInput.vue";
import { Worker, System, errMsg, type AllowedUserInfo, type TokenStatus, type DetectResult } from "../api";
import { store, toast, fail } from "../store";
import { words } from "../platform";
import { ago, idProblem } from "../util";

const st = computed(() => store.status);
const users = ref<AllowedUserInfo[]>([]);
const draft = ref<AllowedUserInfo[]>([]);
const dirty = computed(() => JSON.stringify(users.value) !== JSON.stringify(draft.value));
const tokenStatus = ref<TokenStatus | null>(null);
const saving = ref(false);
const widenAsk = ref(false);

async function load() {
  if (!store.connection.connected) return;
  try {
    users.value = (await Worker.Allowlist()) ?? [];
    if (!dirty.value || !draft.value.length) draft.value = users.value.map((u) => ({ ...u }));
    tokenStatus.value = await Worker.TokenStatus();
  } catch (e) {
    fail(e);
  }
}
watch(() => store.connection.connected, load, { immediate: true });

const newId = ref("");
const newLabel = ref("");
const idErr = computed(() => idProblem(newId.value, draft.value.map((u) => u.id)));
function add() {
  if (idErr.value || !newId.value.trim()) return;
  draft.value.push({ id: Number(newId.value.trim()), label: newLabel.value.trim(), role: "user" });
  newId.value = newLabel.value = "";
}
function remove(i: number) {
  if (draft.value[i].role === "owner") return;
  draft.value.splice(i, 1);
}
function save() {
  if (draft.value.length > users.value.length) widenAsk.value = true;
  else doSave();
}
async function doSave() {
  widenAsk.value = false;
  saving.value = true;
  try {
    users.value = (await Worker.SetAllowlist(draft.value)) ?? [];
    draft.value = users.value.map((u) => ({ ...u }));
    toast("Allowed users saved. Changes apply immediately.", "ok");
  } catch (e) {
    fail(e);
  } finally {
    saving.value = false;
  }
}

// sender detection
const detect = ref<DetectResult | null>(null);
let poll: ReturnType<typeof setInterval> | undefined;
async function startDetect() {
  try {
    detect.value = await Worker.DetectStart(120);
    clearInterval(poll);
    poll = setInterval(async () => {
      detect.value = await Worker.DetectResult();
      if (!detect.value.armed) clearInterval(poll);
    }, 1500);
  } catch (e) {
    fail(e);
  }
}
async function stopDetect() {
  clearInterval(poll);
  detect.value = await Worker.DetectStop().catch(() => null);
}
function pick(id: number, name: string) {
  newId.value = String(id);
  newLabel.value = name.slice(0, 32);
}
onBeforeUnmount(() => clearInterval(poll));

// token replacement
const replacing = ref(false);
const newToken = ref("");
const tested = ref("");
const tokenErr = ref("");
const busy = ref(false);
async function testNew() {
  busy.value = true;
  tokenErr.value = tested.value = "";
  try {
    tested.value = (await Worker.TokenTest(newToken.value.trim())).botUsername;
  } catch (e) {
    tokenErr.value = errMsg(e);
  } finally {
    busy.value = false;
  }
}
async function saveToken() {
  busy.value = true;
  try {
    await Worker.TokenSet(newToken.value.trim());
    newToken.value = "";
    replacing.value = false;
    toast("Token replaced. The worker is restarting to use it. Revoke the old token in @BotFather.", "ok");
  } catch (e) {
    tokenErr.value = errMsg(e);
  } finally {
    busy.value = false;
  }
}
</script>

<template>
  <Page title="Telegram" subtitle="Bot token, who may use the bot, and the connection">
    <div v-if="!store.connection.connected" class="text-muted">Start the worker to manage Telegram settings.</div>
    <template v-else>
      <div class="grid grid-cols-2 gap-3">
        <div class="card px-5 py-4">
          <h2 class="font-semibold">Bot token</h2>
          <dl class="mt-3 grid grid-cols-[8rem_1fr] gap-y-1.5">
            <dt class="text-muted">Bot</dt><dd>{{ tokenStatus?.botUsername ? "@" + tokenStatus.botUsername : "—" }}</dd>
            <dt class="text-muted">Stored in</dt><dd>{{ { keychain: words.tokenStore, file: "private file (.env)", env: "environment variable" }[tokenStatus?.source ?? ""] ?? "—" }}</dd>
            <dt class="text-muted">Token</dt><dd class="mono">{{ tokenStatus?.hint || "—" }}</dd>
          </dl>
          <div class="mt-3 flex gap-2">
            <button class="btn" @click="replacing = true">Replace token…</button>
            <button class="btn btn-ghost" @click="System.Open('botfather')">Open @BotFather</button>
          </div>
          <p v-if="tokenStatus?.source === 'file'" class="mt-3 text-xs text-warn">The token is in a file. Replacing it here moves it into {{ words.tokenStore }}.</p>
        </div>
        <div class="card px-5 py-4">
          <h2 class="font-semibold">Connection</h2>
          <dl class="mt-3 grid grid-cols-[10rem_1fr] gap-y-1.5">
            <dt class="text-muted">Polling</dt><dd><Badge :tone="st?.bot.state === 'polling' ? 'ok' : 'muted'" :label="st?.bot.state ?? '—'" /></dd>
            <dt class="text-muted">Last update</dt><dd>{{ ago(st?.telegram.lastUpdate) }}</dd>
            <dt class="text-muted">Poll / send errors</dt><dd>{{ st?.telegram.pollErrors }} / {{ st?.telegram.sendErrors }}</dd>
            <dt class="text-muted">Ignored senders</dt><dd>{{ st?.telegram.unauthorized }}</dd>
            <dt class="text-muted">Duplicate pollers</dt><dd :class="(st?.telegram.conflicts409 ?? 0) > 0 ? 'text-danger font-semibold' : ''">{{ st?.telegram.conflicts409 }}</dd>
          </dl>
          <p v-if="(st?.telegram.conflicts409 ?? 0) > 0" class="mt-2 text-xs text-danger">Another program polls this bot: a second copy, or a leaked token. Replace the token if it is not yours.</p>
        </div>
      </div>

      <div class="card mt-4">
        <div class="flex items-center justify-between px-5 pt-4">
          <div>
            <h2 class="font-semibold">Allowed Telegram users</h2>
            <p class="mt-0.5 text-xs text-muted">Numeric IDs only. Labels are reminders for you; they never grant access. Each person uses the bot in their own private chat.</p>
          </div>
          <button class="btn btn-primary" :disabled="!dirty || saving" @click="save">{{ saving ? "Saving…" : "Save changes" }}</button>
        </div>
        <table class="tbl mt-3">
          <thead><tr><th>ID</th><th>Label</th><th>Role</th><th class="w-24"></th></tr></thead>
          <tbody>
            <tr v-for="(u, i) in draft" :key="u.id">
              <td class="mono">{{ u.id }}</td>
              <td><input v-model="u.label" class="input py-1" maxlength="32" :aria-label="`Label for ${u.id}`" /></td>
              <td>
                <span v-if="u.role === 'owner'" class="font-medium">Owner</span>
                <select v-else v-model="u.role" class="input py-1" :aria-label="`Role for ${u.id}`"><option value="user">User</option></select>
              </td>
              <td class="text-right">
                <button v-if="u.role !== 'owner'" class="btn btn-danger btn-sm" @click="remove(i)">Remove</button>
                <span v-else class="text-xs text-muted" title="The owner cannot be removed here">locked</span>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="flex items-end gap-2 border-t border-line px-5 py-3">
          <div><label for="nid" class="label">Add ID</label><input id="nid" v-model="newId" class="input mono mt-1 w-44" inputmode="numeric" @keydown.enter="add" /></div>
          <div class="flex-1"><label for="nlabel" class="label">Label</label><input id="nlabel" v-model="newLabel" class="input mt-1" maxlength="32" @keydown.enter="add" /></div>
          <button class="btn" :disabled="!newId || !!idErr" @click="add">Add</button>
          <button v-if="!detect?.armed" class="btn btn-ghost" @click="startDetect">Detect from a message…</button>
          <button v-else class="btn btn-ghost" @click="stopDetect">Stop detecting</button>
        </div>
        <p v-if="idErr" class="px-5 pb-3 text-danger">{{ idErr }}</p>
        <div v-if="detect?.armed || detect?.senders?.length" class="border-t border-line px-5 py-3">
          <p class="text-muted">
            <template v-if="detect.armed">Listening for 2 minutes: ask the person to send any message to the bot. Nobody gets a reply and nothing is logged.</template>
            <template v-else>Detection ended.</template>
          </p>
          <ul class="mt-2 space-y-1">
            <li v-for="x in detect.senders ?? []" :key="x.id" class="flex items-center gap-3">
              <span class="mono">{{ x.id }}</span> <span>{{ x.firstName }}</span> <span v-if="x.username" class="text-muted">@{{ x.username }}</span>
              <button class="btn btn-sm" @click="pick(x.id, x.firstName)">Use</button>
            </li>
          </ul>
        </div>
      </div>
      <p class="mt-3 text-xs text-muted">Users with the <b>user</b> role can chat, manage sessions and their own jobs. Tiers, defaults, screens, the machine and deleting history stay owner-only.</p>
    </template>

    <Modal v-if="widenAsk" title="Allow another person?" @close="widenAsk = false">
      <p>New users will be able to send prompts to Claude Code on {{ words.thisComputer }}, within the tier of the session they use, and to see the bot's shared sessions. Only add people you trust with this computer.</p>
      <template #actions>
        <button class="btn" @click="widenAsk = false">Cancel</button>
        <button class="btn btn-primary" @click="doSave">Allow and save</button>
      </template>
    </Modal>

    <Modal v-if="replacing" title="Replace the bot token" @close="replacing = false">
      <p class="text-muted">Get a new token from @BotFather (<span class="mono">/token</span>). It is tested with Telegram before anything changes; the worker then restarts to use it.</p>
      <label for="newtok" class="label mt-4 block">New token</label>
      <div class="mt-1.5"><SecretInput id="newtok" v-model="newToken" /></div>
      <div class="mt-2 flex items-center gap-2">
        <button class="btn btn-sm" :disabled="!newToken || busy" @click="testNew">Test</button>
        <Badge v-if="tested" tone="ok" :label="'@' + tested" />
      </div>
      <p v-if="tokenErr" class="mt-2 text-danger" role="alert">{{ tokenErr }}</p>
      <template #actions>
        <button class="btn" @click="replacing = false">Cancel</button>
        <button class="btn btn-primary" :disabled="!tested || busy" @click="saveToken">Save and restart worker</button>
      </template>
    </Modal>
  </Page>
</template>
