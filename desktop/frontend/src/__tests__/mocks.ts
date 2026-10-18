// Fixtures and binding mocks for rendering every view without the Go side.
// vi.mock("../api", ...) in a test file uses `api` below.
import words from "./words.json";
import type { OS } from "./forbidden";

export function platform(os: OS) {
  const w = (words as Record<string, any>)[os];
  return {
    os, arch: "amd64", osName: w.osName, version: "1", pretty: w.osName + " 1",
    session: os === "darwin" ? "aqua" : os === "windows" ? "windows" : "x11", desktop: os === "linux" ? "GNOME" : "",
    headless: false, distroId: os === "linux" ? "ubuntu debian" : "", words: w,
  };
}

const now = Math.floor(Date.now() / 1000);
const session = { id: 1, label: "S1", name: "main", workspace: "app", mode: "observe", model: "fable", effort: "auto", modelSet: false, effortSet: false,
  status: "active", origin: "telegram", claudeShort: "abcd", started: true, turns: 3, createdAt: now - 600, lastUsedAt: now - 60, active: true };
const workspace = { name: "app", path: "/work/app", defaultMode: "observe", allowedCommands: ["Bash(go test *)"], deniedCommands: ["Bash(rm *)", "Bash(sudo *)", "Bash(git push *)"], iKnowWhatIAmDoing: false, isDefault: true };

export const fixtures: Record<string, any> = {
  Connection: { connected: true, configured: true },
  Status: { version: "1.0.0", pid: 42, startedAt: now - 3600, bot: { state: "polling", since: now - 3600, pollStarts: 1 },
    telegram: { pollErrors: 0, sendErrors: 0, conflicts409: 0, unauthorized: 0, allowedUsers: 1, tokenStore: "keychain", tokenHint: "…AAAA", botUsername: "testbot" },
    claude: { binary: "/bin/claude", version: "2.1.302", loggedIn: true, authMethod: "claude.ai", subscription: "max" },
    active: session, jobs: { running: 1, queued: 0, today: 4 }, lastErrorAt: 0, screen: { configured: true, enabled: true } },
  Sessions: [session],
  ForgottenSessions: [],
  Jobs: [{ id: 7, label: "j-7", sessionId: 1, state: "completed", prompt: "hello", createdAt: now - 120, startedAt: now - 119, finishedAt: now - 100, model: "fable", effort: "auto", mode: "observe", inTokens: 10, outTokens: 20, costUsd: 0.01 }],
  Processes: { owned: [], agents: [], agentsAt: now, claudeCount: 1, workerRssKB: 50000, system: { load1: 1, load5: 1, load15: 1, hasLoad: true, memTotal: 16e9, memFree: 8e9 } },
  Models: { defaultModel: "fable", defaultEffort: "auto", builtin: ["fable", "opus", "sonnet", "haiku"], effortLevels: ["low", "medium", "high", "xhigh", "max", "auto"], aliases: [] },
  Diagnostics: [
    { id: "worker", label: "Background worker", ok: true, detail: "clawdyx 1.0.0" },
    { id: "camera", label: "Camera", ok: false, detail: "permission not granted", fix: "Fix it.", fixKey: "camera" },
  ],
  LogTail: [{ time: "10:00", level: "info", msg: "bot started", attrs: {} }],
  Screens: { status: { configured: true, enabled: true }, displays: [{ index: 1, key: "AAAAAAAA", name: "Main", main: true, builtin: true, pixelW: 1920, pixelH: 1080, pointW: 1920, pointH: 1080, scale: 1 }], access: "denied", held: 0, capturesToday: 1 },
  Allowlist: [{ id: 111111111, label: "me", role: "owner" }],
  TokenStatus: { source: "file", hint: "…AAAA", botUsername: "testbot" },
  Settings: {
    configPath: "/cfg/config.json", ownerUserId: 111111111, workspaces: [workspace], defaultWorkspace: "app", pollTimeoutSeconds: 30,
    claudeBinary: "/bin/claude", settingSources: ["user"], strictMcpConfig: true, gitBashPath: "", defaultModel: "fable", defaultEffort: "auto",
    maxConcurrentJobs: 2, jobTimeoutMinutes: 15, maxTurns: 60, maxBudgetUsd: 0, maxQueuePerSession: 5, maxOutputBytes: 1048576, maxChildRssMb: 2048,
    bgWaitCeilingMs: 60000, childPath: ["/usr/bin", "/bin"], childPathDefault: ["/usr/bin", "/bin"], extraDisallowedTools: [], builtinDenied: ["Bash(rm *)"],
    automationEnabled: false, screenControl: true, camera: false, cameraSupported: true, openUrls: true, browserApp: "Firefox", shutdownCommand: false,
    stateDir: "/state", logDir: "/logs", logLevel: "info", jobsDays: 90, jobEventsDays: 14, auditDays: 180, processedUpdatesDays: 7, restartPending: true,
  },
  Info: { version: "1.0.0", configPath: "~/cfg/config.json", logDir: "~/logs", stateDir: "~/state", launchAtLogin: false, inBundle: true, trayAvailable: true },
  Service: { installed: true, loaded: true, pid: 42, binary: "~/clawdyx", appWorker: "~/clawdyx", usesAppWorker: true, inBundle: true },
  InstalledBrowsers: ["Firefox"],
  HasLink: true,
  HelpTopics: [{ id: "getting-started", title: "Getting started" }, { id: "terminal", title: "Terminal setup" }],
  Help: { id: "getting-started", title: "Getting started", html: "<h1>Getting started</h1><p>The bot runs in a small background worker. See <a href=\"help:terminal\">Terminal setup</a> or <a href=\"open:releases\">the release page</a>.</p>" },
  State: { configExists: false, tokenStored: false, workerFound: true, claudeBinary: "/bin/claude", browser: "Firefox", suggestFolder: "/work", configPath: "~/cfg/config.json" },
};

// Every binding call resolves with its fixture (undefined when there is none).
const service = () => new Proxy({}, { get: (_t, name: string) => () => Promise.resolve(fixtures[name]) });

export const api = {
  Worker: service(),
  System: service(),
  Setup: service(),
  errMsg: (e: unknown) => String(e),
};
