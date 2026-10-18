# Testing

```sh
make test               # go test -race ./...  (no network, no real claude; without gcc on Windows, no -race)
make lint               # gofmt, go vet (+ staticcheck/govulncheck if installed)
make check-secrets      # fails if a token-shaped string is tracked
make check-wording      # fails if user-facing text sends people to the settings file instead of the app
make check-docs         # fails if a per-OS section of README, RUNBOOK or PRIVACY names another OS
make test-desktop       # desktop module (Go) + frontend type check and Vitest
make vet-windows        # type-checks both modules, tests included, for Windows
make vet-linux          # the same for Linux (the app's cgo part only on Linux)
make vet-darwin         # the same for macOS (the app's cgo part only on a Mac)
make check-all          # all of the above
```

Every system's code is compiled and vetted on every `make check-all`, whatever the host. The tests themselves run on the host's OS; CI runs them on all three.

## CI

`.github/workflows/ci.yml` runs on every push to `main` and every pull request:

| Job | Runners | Steps |
| --- | --- | --- |
| `go` | macos-14, windows-latest, ubuntu-24.04 | `go vet`, `go test -race`, gofmt, and `go vet` of both modules for the other two OSes |
| `desktop` | the same three | frontend type check, Vitest and build; desktop module vet and test; the app built for the runner's OS (Linux: GTK 3 + WebKitGTK 4.1, started under `xvfb` for 5 s); Linux: the launcher entry validated, `.deb`/`.rpm`/`.tar.gz` built, the `.deb` installed and the worker run from it; Windows: the camera helper built with MSVC and its `list` checked, the installer built, installed silently, checked and removed on the runner |
| `secrets` | ubuntu-24.04 | `make check-secrets` over the history |

## What the suites cover

| Package | Coverage |
| --- | --- |
| `internal/platform` | `Detect` for every Linux session kind (X11, Wayland, headless) from environment overrides; os-release parsing; every `Words` field filled for every OS; no `runtime.GOOS` outside the package; the source scan (string literals outside OS files name no OS product); the forbidden-word matcher; the frontend fixtures equal the Go tables; the per-OS sections of the docs |
| `internal/config` | Allowlist: exactly one owner, positive unique IDs, roles, labels, size cap; `Rewrite` validates before replacing and loads an invalid file leniently; 0600 checks; token format without echoing; symlinked workspace canonicalised; case-insensitive duplicate workspaces on macOS and Windows; minimum deny list; folders per OS; the Windows 0.4.x migration (files moved, token re-keyed, never overwrites, skipped with `CLAWDYX_CONFIG`); the `.env` follows `-config`; Git Bash validation; 0.4.1 settings files for macOS and Windows load, and a 1.0 rewrite adds no key 0.4.1 rejects unless `git_bash_path` is set |
| `internal/settings` | every key: info, apply, reset per section, export without the token, import with a diff and the owner-change guard |
| `internal/observability` | Redaction of bot tokens (including inside `/bot<token>/` URLs), Anthropic keys, bearer headers, key=value secrets, emails; logger output redacted |
| `internal/sessions` | `/newf` in one transaction, name uniqueness, `/clear` copies tier/model/effort, forget/restore window, restore refused after history deletion; history lookup, ambiguous and symlinked transcripts refused, only the session's own two paths deleted |
| `internal/store` | v1→v2 and v3→v4 migrations; tombstones; capture audit; a schema v4 database written by 0.4.1 opens without migration |
| `internal/policy` | observe/assisted/automation flag mapping per OS against golden files (`golden/<os>-<tier>.txt`), the per-OS deny lists and secret paths, Windows' PowerShell tool removed, bypass and automation unreachable, broad allow rules rejected, path traversal, symlink escape, workspace swap |
| `internal/agent` | No prompt in argv, variadic flags last, env allowlist per OS (no token, API key, nested markers), stream-json parsing, usage dedupe, denials, SIGINT records a result, escalation kills grandchildren, PID-reuse guard (`/proc` stat parsing on Linux), missing transcript, model probe |
| `internal/processes` | sampling per OS, `/proc/loadavg` and `/proc/meminfo` parsing, CPU from lifetime counters, no load line where the OS has none |
| `internal/jobs` | Per-session serialisation, positions, cancel queued/waiting/running, queue limit, concurrency cap, timeout, failure mapping, pause, shutdown, recovery kills owned orphans and never re-runs; the screen system prompt per OS |
| `internal/telegram` | Roles, silent rejection, lifecycle commands and confirmations, control-socket methods, camera and screen commands with fake providers (never at startup, only to the owner, `/screen off`), settings through the socket (set, reset, export, import), `/help`, `/doctor` and replies rendered with each OS's words contain no word of another OS, the Linux session check |
| `internal/control` | Socket 0600 in a 0700 dir, peer-uid rejection (macOS, Linux), typed errors, takeover refused, oversize and garbage frames, ordered events, backpressure; `FuzzFrames` |
| `internal/camera` | ordering and selection, clip length 1–60 s, the switch, audit, five-minute hold, the helper JSON protocol (fake helpers on macOS and Windows), the capture folder per OS and its cleanup; Linux: V4L2 struct sizes and ioctl numbers (compile-time and test), YUYV → JPEG conversion, no camera → empty list, a fake `ffmpeg` writes only into a private folder that is gone afterwards |
| `internal/screen` | display ordering and keys, JPEG encoding, broker, MCP tools (coordinates on a second display, the worker's authorization, the session and input availability in `list_displays`, a Wayland-like desktop refusing input); Windows: DPI awareness, monitor enumeration, capture of the real desktop, key mapping; X11 helpers (EDID names, keysyms, pixel order) |
| `internal/vault` | Memory store semantics, per-config accounts, a missing token store reported as unavailable (the file fallback). Tests never touch the real token store (`TestMain` swaps in a memory store). |
| `cmd/clawdyx` | Run-key command quoting; the supervisor's backoff; the systemd unit text; setup (vault by default, `-token-file`, `-migrate-token`, no keyring → file, `-headless`, the next steps per OS); `paths`; `token delete` removes only this configuration's token |
| `desktop` (Go) | host per OS (service status, worker locations, links), Help (every topic × OS × Linux session: forbidden words, links, render time), the update check (asset per OS, architecture and distribution; foreign URLs ignored), the diagnostics report (system line, redaction, no Telegram IDs when copied), notifications (only when hidden and on, no text, rate limit), the second-launch wiring, settings offline, every bound method exists |
| `desktop/frontend` (Vitest) | every view mounted for macOS, Windows and Linux names no other OS; Help (topics, search, only `help:`/`open:` links followed); About (system line, service, folders, the download of an update); Settings sections; helpers |
| `internal/setup` | Owner detection; workspace and name checks; Git Bash detection |
| `internal/render` | Escaping, Markdown conversion, safe links only, chunk limits |

Unit and end-to-end tests use `internal/agent/testdata/fakeclaude`, a small program that imitates `claude -p` stream-json, and an `httptest` fake of the Bot API.

## Live tests (real claude CLI, small cost)

```sh
CLAWDYX_LIVE_DIR=$(mktemp -d) go test -tags live -run TestLive -v ./internal/jobs/
```

These run short prompts against the installed CLI with the bot's real flags and verify: reads inside the workspace work and writes are denied (observe); reads outside are blocked for both the Read tool and Bash `cat`; file creation works and `rm` is denied (assisted); context survives a model switch. `TestLiveAutomation` checks the automation tier; `TestLiveScreenTools` (with `CLAWDYX_BIN=<worker>`) checks that the real CLI starts the bot's screen MCP server and calls the screenshot tool (one image of the main display goes to Claude).

Last full run on macOS: 2026-10-16, CLI 2.1.292. On Windows the permission rules below were verified on 2026-10-18 with CLI 2.1.302.

### Permission rules on Windows

The Windows built-in deny list relies on how Claude Code applies rules there; repeat this after a Claude Code update (results: `docs/ARCHITECTURE.md` §3.1). In PowerShell, in an empty scratch folder with a subfolder `x`:

```powershell
$deny = "Bash(rm *)", "PowerShell", "WebFetch"
"Use your shell tool to run exactly: echo start && rm -rf ./x && echo done" | claude -p --model haiku --permission-mode auto --output-format json --max-turns 4 --strict-mcp-config --disallowedTools $deny
```

Expect `permission_denials` with the whole command and `x` still there. Then run the same without `"Bash(rm *)"` (the control: `x` is deleted), the assisted shape (`--permission-mode acceptEdits --allowedTools "Bash(echo *)"`), a `Read` rule in Git Bash form (`Read(//c/Users/<you>/<folder>/**)`) against both the Read tool and `cat /c/Users/<you>/<folder>/file`, and check with `--output-format stream-json --verbose` that `system/init.tools` contains `PowerShell` without the rule and not with it.

## Testing next to a running bot

On a computer whose own bot must keep running, use an isolated instance: its own settings file (`CLAWDYX_CONFIG=<folder>/config.json` or `-config`, which also moves the token item, the `.env`, the state and the logs), its own service name (`CLAWDYX_SERVICE_LABEL=local.clawdyx.dev`), a separate test bot token, and a development build of the app with its own single-instance id (`-ldflags "-X main.instanceID=…dev"`). Stop only the processes whose pids the test instance recorded; installers belong in Windows Sandbox or a VM.

## Per-OS QA checklists (real hardware)

Each checklist runs on the release candidate; the outcome ("tested on …") goes into the release notes.

**macOS 14/15 (Apple silicon)** — regression: install the dmg over 0.4.1; wizard on a clean account; Start/Stop; one prompt; `/screen`, `/look`, a Claude screen tool in automation; `/camera`, `/camera video 5`; Quit dialog; reopen; Settings: every section saves; Help renders without Windows or Linux words; the update check names the dmg.

**Windows 10 22H2 and Windows 11** (standard user; one account with a non-ASCII name and a space; one machine without Git for Windows; one with camera access off):

1. Installer: no UAC prompt; Start menu entry; SmartScreen "More info → Run anyway" as documented.
2. Wizard: the Credential Manager item appears; settings under `%APPDATA%\clawdyx`; the Run value points at `clawdyxw.exe`.
3. Login: no console window flashes; `clawdyx service status` shows supervisor and worker pids; end the worker in Task Manager → back within 15 s.
4. Upgrade from the 0.4.1 layout (`%USERPROFILE%\.config`): the settings move, the token is found, the bot answers.
5. Telegram: a prompt, `/processes` (CPU during a job, no load line), `/doctor` (Git Bash row), `/open`.
6. Screens: two monitors with different scaling; `/screens`, `/screen 2`, `/look`, the app's Monitors; automation: Notepad open, click, type Cyrillic, `ctrl+s`; a click into an elevated window → the documented error.
7. Camera: `/cameras`, `/camera`, `/camera video 10`; with camera access off → the permission reply and the Diagnostics button opens the camera settings; the helper refuses when started from PowerShell.
8. Deny list: `powershell -c Get-Date` denied in assisted and automation.
9. Settings: every section; reset; export/import; editing with the worker stopped.
10. Help, About (system row), the update check (names the setup.exe), notifications.
11. A workspace path over 260 characters.
12. Uninstall with the purge: nothing left in `%LOCALAPPDATA%\Programs\clawdyX`, `%APPDATA%\clawdyx`, `%LOCALAPPDATA%\clawdyx`, the Credential Manager and the Run key.

**Linux, GNOME Wayland (Ubuntu 24.04):** `.deb` install; launcher icon; no tray → the one-time hint, closing the window quits; the wizard stores the token in GNOME Keyring (`secret-tool search service clawdyx`); the unit is active and survives `kill -9`; `/screen` through the portal (note whether a dialog appears per capture); input tools return the Wayland message; `/camera` photo; with `ffmpeg` a clip; Settings, Help and About wording; the update check names the `.deb`.

**Linux, KDE Plasma (Fedora, Wayland):** `.rpm`; tray present; portal screenshot; otherwise as above.

**Linux, X11 session (Ubuntu on Xorg or Xubuntu):** the tarball; `/screens` lists the outputs; `/screen`, `/look`; automation types Cyrillic into a text editor and presses `ctrl+s`; the keyboard mapping is restored afterwards (`xmodmap -pke` unchanged).

**Linux server (Ubuntu Server 24.04, Debian 12, SSH only):** the worker tarball; `clawdyx setup -headless -token-stdin …`; the token in `.env` with the printed reason; `clawdyx service start` and linger; the bot answers; after a reboot it is back without a login; `/doctor` names no graphical session; `/screens` and `/camera` explain themselves; `journalctl --user -u clawdyx` shows the start.

### What needs hardware, what CI covers

| Needs hardware | CI covers |
| --- | --- |
| Console flash at login; Task Manager kill and restart; SmartScreen; input into elevated windows; a Windows camera; per-monitor scaling; `ms-settings:` links | compile and vet for three OSes; unit and golden tests; Windows desktop capture on the runner; the MSVC helper build; the installer installed and removed |
| Wayland portal dialogs; GNOME without a tray; a locked keyring after SSH; a V4L2 device; ffmpeg encoding; linger | the app started under `xvfb`; the `.deb` installed and run |
| macOS TCC prompts; notarization | everything else |

## Manual acceptance (needs a real bot token)

1. From the owner account, `/start`, `/status`, a plain prompt, `/model fable`, `/effort high`, `/stop` during a long prompt.
2. From a second Telegram account, send messages and press old buttons. Expect no reply and no new rows in `/jobs`.
3. `/screens`, `/screen 2`, `/look 2 what is on this screen?`, `/screen off`, then `/screen 2` again (expect the switched-off message).
4. Add the bot to a group and send `/status` there as the owner. Expect no reply.
5. Stop the worker during a long job (`clawdyx service stop`, or end its process) and start it again. Expect the "interrupted, not re-run" notice and no duplicate job.
6. Start a second `clawdyx run`. Expect exit code 3 with "another instance".
