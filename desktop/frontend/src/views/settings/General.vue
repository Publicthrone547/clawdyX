<script setup lang="ts">
import { onMounted, ref } from "vue";
import Section from "./Section.vue";
import { System } from "../../api";
import { toast, fail } from "../../store";
import { theme, setTheme, notifications, setNotificationsPref, type Theme } from "../../prefs";
import { words } from "../../platform";
import { errMsg } from "../../api";

const th = ref<Theme>(theme());
const launch = ref(false);
const notify = ref(notifications());
function setNotify(on: boolean) {
  notify.value = on;
  setNotificationsPref(on);
  System.SetNotifications(on).catch(fail);
}
const trayAvailable = ref(true);
onMounted(async () => {
  try {
    const i = await System.Info();
    launch.value = i.launchAtLogin;
    trayAvailable.value = i.trayAvailable;
  } catch (e) {
    fail(e);
  }
});
async function setLaunch(on: boolean) {
  try {
    launch.value = await System.SetLaunchAtLogin(on);
  } catch (e) {
    toast(errMsg(e), "danger");
  }
}
</script>

<template>
  <Section title="General">
    <div class="mt-3 grid grid-cols-2 gap-4">
      <div>
        <label class="label" for="theme">Appearance</label>
        <select id="theme" v-model="th" class="input mt-1" @change="setTheme(th)"><option value="system">Follow system</option><option value="light">Light</option><option value="dark">Dark</option></select>
      </div>
      <div>
        <span class="label">The app at login</span>
        <label class="mt-2 flex items-center gap-2"><input type="checkbox" :checked="launch" @change="setLaunch(($event.target as HTMLInputElement).checked)" /> {{ words.launchAtLogin }}</label>
        <p class="mt-1 text-xs text-muted">The worker starts at login on its own; this is only the window and the {{ words.trayName }}.</p>
      </div>
      <div>
        <span class="label">Notifications</span>
        <label class="mt-2 flex items-center gap-2"><input type="checkbox" :checked="notify" @change="setNotify(($event.target as HTMLInputElement).checked)" /> Show notifications</label>
        <p class="mt-1 text-xs text-muted">While the window is hidden: when a job ends, or the worker stops. They name the job, never its text.</p>
      </div>
    </div>
    <p v-if="!trayAvailable" class="mt-3 text-xs text-muted">Your desktop shows no tray icons, so closing the window quits the app. The worker keeps running; open the app again from the launcher.</p>
  </Section>
</template>
