<script setup lang="ts">
import { onMounted, ref } from "vue";
import Section from "./Section.vue";
import { System } from "../../api";
import { store } from "../../store";
import { words, isMac } from "../../platform";
import { st } from "./state";

// The /open browser is a macOS application; elsewhere /open uses the
// default browser and there is nothing to choose.
const browsers = ref<string[]>([]);
onMounted(async () => {
  if (isMac.value) browsers.value = (await System.InstalledBrowsers().catch(() => [])) ?? [];
});
const headless = () => !!store.platform?.headless;
</script>

<template>
  <Section title="Features" section="features" hint="What you and Claude may do beyond working in the project folders.">
    <div class="mt-3 space-y-2">
      <label class="flex items-start gap-2"><input v-model="st.draft.automationEnabled" type="checkbox" class="mt-0.5" /><span><b>Automation tier available</b><span class="block text-xs text-muted">Lets a session switch to automation: Claude runs commands that Claude Code's safety classifier approves, anywhere on {{ words.thisComputer }}. Each session still needs an explicit switch.</span></span></label>
      <label v-if="!headless()" class="flex items-start gap-2"><input v-model="st.draft.screenControl" type="checkbox" class="mt-0.5" /><span><b>Screen features</b><span class="block text-xs text-muted">Monitors, /screen and /look, and Claude's screenshot, mouse and keyboard tools in automation sessions. {{ words.screenNeeds }}</span></span></label>
      <p v-else class="text-xs text-muted"><b>Screen features</b> are not available: there is no graphical session on {{ words.thisComputer }}.</p>
      <label v-if="st.s?.cameraSupported" class="flex items-start gap-2"><input v-model="st.draft.camera" type="checkbox" class="mt-0.5" /><span><b>Camera</b><span class="block text-xs text-muted">Owner-typed /cameras, /camera and /camera video in your private chat: a photo or a clip of up to 60 seconds without sound. Claude never gets the camera. Switched off together with the screen features by /screen off.</span></span></label>
      <p v-else class="text-xs text-muted"><b>Camera</b> is not available on {{ words.thisComputer }} in this version.</p>
      <label class="flex items-start gap-2"><input v-model="st.draft.openUrls" type="checkbox" class="mt-0.5" /><span><b>/open in Telegram</b><span class="block text-xs text-muted">Owner-typed command that opens a website in {{ isMac ? "the browser below" : "the default browser" }} on {{ words.thisComputer }}.</span></span></label>
      <div v-if="isMac" class="ml-6 w-64">
        <input v-model="st.draft.browserApp" class="input" list="browsers" aria-label="Browser for /open" />
        <datalist id="browsers"><option v-for="b in browsers" :key="b" :value="b" /></datalist>
      </div>
      <label class="flex items-start gap-2"><input v-model="st.draft.shutdownCommand" type="checkbox" class="mt-0.5" /><span><b>/shutdown in Telegram</b><span class="block text-xs text-muted">Lets the owner stop the worker from Telegram. Starting it again then needs {{ words.thisComputer }}.</span></span></label>
    </div>
  </Section>
</template>
