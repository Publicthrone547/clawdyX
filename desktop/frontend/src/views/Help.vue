<script setup lang="ts">
// Help: topics rendered by the Go side for this OS (offline, embedded in
// the app). Links are help:<topic> or open:<key>; nothing else is followed.
import { computed, onMounted, ref } from "vue";
import Page from "../components/Page.vue";
import { System, errMsg, type D } from "../api";
import { store, fail } from "../store";

const topics = ref<D.HelpTopic[]>([]);
const page = ref<D.HelpPage | null>(null);
const query = ref("");
const error = ref("");

const shown = computed(() => {
  const q = query.value.trim().toLowerCase();
  return q ? topics.value.filter((t) => t.title.toLowerCase().includes(q)) : topics.value;
});

async function show(id: string) {
  try {
    page.value = await System.Help(id);
    store.helpTopic = id;
    error.value = "";
  } catch (e) {
    error.value = errMsg(e);
  }
}

async function onClick(e: MouseEvent) {
  const a = (e.target as HTMLElement | null)?.closest("a");
  if (!a) return;
  e.preventDefault();
  const href = a.getAttribute("href") ?? "";
  if (href.startsWith("help:")) {
    await show(href.slice(5));
  } else if (href.startsWith("open:")) {
    const key = href.slice(5);
    if (await System.HasLink(key)) System.Open(key).catch(fail);
  }
}

onMounted(async () => {
  topics.value = (await System.HelpTopics()) ?? [];
  const first = topics.value.find((t) => t.id === store.helpTopic) ?? topics.value[0];
  if (first) await show(first.id);
});
</script>

<template>
  <Page title="Help" :subtitle="`How clawdyX works on ${store.platform?.words.thisComputer ?? 'this computer'}`">
    <div class="flex gap-6">
      <aside class="w-52 shrink-0">
        <input v-model="query" class="input w-full" type="search" placeholder="Search topics" aria-label="Search help topics" />
        <ul class="mt-3 flex flex-col gap-0.5" aria-label="Help topics">
          <li v-for="t in shown" :key="t.id">
            <button
              class="w-full rounded-md px-3 py-1.5 text-left"
              :class="page?.id === t.id ? 'bg-surface-2 font-semibold' : 'text-muted hover:bg-surface-2'"
              :aria-current="page?.id === t.id ? 'page' : undefined"
              @click="show(t.id)"
            >
              {{ t.title }}
            </button>
          </li>
          <li v-if="!shown.length" class="px-3 py-1.5 text-xs text-muted">No topic matches.</li>
        </ul>
      </aside>
      <!-- eslint-disable-next-line vue/no-v-html -- Markdown rendered by the app itself, raw HTML dropped -->
      <article v-if="page" class="help card min-w-0 flex-1 px-6 py-5" @click="onClick" v-html="page.html" />
      <p v-else-if="error" class="text-danger">{{ error }}</p>
    </div>
  </Page>
</template>

<style scoped>
.help :deep(h1) { font-size: 1.25rem; font-weight: 600; margin-bottom: .75rem; }
.help :deep(h2) { font-size: 1rem; font-weight: 600; margin: 1.25rem 0 .5rem; }
.help :deep(p), .help :deep(ul), .help :deep(ol), .help :deep(table), .help :deep(pre) { margin: .5rem 0; line-height: 1.55; }
.help :deep(ul) { list-style: disc; padding-left: 1.25rem; }
.help :deep(ol) { list-style: decimal; padding-left: 1.25rem; }
.help :deep(li) { margin: .2rem 0; }
.help :deep(a) { color: var(--accent); text-decoration: underline; cursor: pointer; }
.help :deep(code) { font-family: var(--font-mono); font-size: .8em; background: var(--surface-2); border-radius: 4px; padding: 0 .25rem; }
.help :deep(pre) { background: var(--surface-2); border-radius: 8px; padding: .75rem 1rem; overflow-x: auto; }
.help :deep(pre code) { background: none; padding: 0; }
.help :deep(table) { border-collapse: collapse; width: 100%; font-size: .9em; }
.help :deep(th), .help :deep(td) { border: 1px solid var(--border); padding: .35rem .6rem; text-align: left; vertical-align: top; }
.help :deep(th) { background: var(--surface-2); font-weight: 600; }
</style>
