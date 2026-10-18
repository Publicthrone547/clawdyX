// The OS the app runs on and its words. Views never spell an OS name or an
// OS-specific product themselves: they read it from here (the Go side's
// internal/platform is the single source).
import { computed } from "vue";
import { store } from "./store";
import type { D } from "./api";

export type Words = D.PlatformInfo["words"];

// neutral is shown until the platform is known (a fraction of a second at
// start); it names no OS.
export const neutral: Words = {
  os: "",
  osName: "your system",
  thisComputer: "this computer",
  theComputer: "the computer",
  settingsApp: "the system settings",
  fileManager: "file manager",
  terminal: "a terminal",
  shell: "shell",
  trayName: "tray icon",
  tokenStore: "the system token store",
  tokenStoreShort: "token store",
  noKeyringNote: "The system token store is not available; the token is kept in a private file instead.",
  serviceKind: "background service",
  serviceLoaded: "registered",
  launchAtLogin: "Start at login",
  shortcutMod: "",
  installClaude: "",
  startAgainHint: "",
  stopServiceHint: "",
  startNeedsHint: "",
  serviceCommands: "",
  setupNext: "",
  deniedSummary: "",
  screenPermission: "",
  inputPermission: "",
  cameraPermission: "",
  screenNeeds: "",
  screenRiskNote: "",
  cameraRiskNote: "",
  updateHint: "Download the new version from the release page.",
  homeExample: "",
};

/** words is the vocabulary of the running OS. */
export const words = computed<Words>(() => store.platform?.words ?? neutral);

export const isMac = computed(() => store.platform?.os === "darwin");
export const isWindows = computed(() => store.platform?.os === "windows");
export const isLinux = computed(() => store.platform?.os === "linux");
