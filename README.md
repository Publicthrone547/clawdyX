<p align="center">
  <img src="desktop/build/appicon.png" alt="clawdyX app icon" width="112" height="112">
</p>

<h1 align="center">clawdyX</h1>

<p align="center"><b>Control Claude Code on your computer from Telegram.</b><br>A private bot and a desktop control panel for the Claude Code you already use.</p>

<p align="center">
  <a href="https://github.com/Publicthrone547/clawdyX/releases/latest"><img alt="Download" src="https://img.shields.io/badge/Download-c2410c?style=for-the-badge&logo=github&logoColor=white" height="44"></a>
</p>

<p align="center">
  <a href="https://github.com/Publicthrone547/clawdyX/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/Publicthrone547/clawdyX?style=flat-square&label=latest%20release&color=c2410c"></a>
  <a href="https://github.com/Publicthrone547/clawdyX/releases"><img alt="Downloads" src="https://img.shields.io/github/downloads/Publicthrone547/clawdyX/total?style=flat-square&label=downloads&color=2f2f33"></a>
  <img alt="Platforms: macOS 14+, Windows 10/11, Linux" src="https://img.shields.io/badge/platforms-macOS%2014%2B%20%C2%B7%20Windows%2010%2F11%20%C2%B7%20Linux-111111?style=flat-square">
  <a href="LICENSE"><img alt="License: PolyForm Noncommercial 1.0.0" src="https://img.shields.io/badge/license-PolyForm%20Noncommercial-555555?style=flat-square"></a>
</p>

<p align="center">
  <a href="https://github.com/Publicthrone547/clawdyX/releases/latest"><b>Latest release</b></a> ·
  <a href="https://github.com/Publicthrone547/clawdyX/releases"><b>All releases</b></a> ·
  <a href="CHANGELOG.md"><b>What's new</b></a> ·
  <a href="#install">Install guide</a>
</p>

---

## What it is

A private, single-operator Telegram bot that drives the **Claude Code CLI already installed and signed in on your computer**. It runs `claude -p` as a child process per message (stream-json output, `--resume` for continuity), so it reuses your existing claude.ai login. It never uses an API key and never bypasses Claude Code's permission system.

Only the Telegram users on your local allowlist are served (by numeric ID, in their own private chat): you as the owner, plus anyone you deliberately add. Every other update is dropped silently before any lookup or side effect.

clawdyX runs on **macOS, Windows and Linux** with the same features; the few differences come from the operating system and are listed where they matter.

| | macOS | Windows | Linux |
| --- | --- | --- | --- |
| Versions | 14 or later, Apple silicon | 10 (1803+) and 11, x64 (arm64: untested) | 64-bit with systemd; the app needs GTK 3 and WebKitGTK 4.1 |
| Install | `.dmg` | per-user installer or `.zip` | `.deb`, `.rpm`, AppImage or `.tar.gz` |
| Background worker | LaunchAgent | login entry with a supervisor | systemd user service |
| Bot token | Keychain | Credential Manager | Secret Service keyring, or a private file |
| Screens and Claude's mouse and keyboard tools | yes (permissions) | yes | X11: yes · Wayland: screenshots only · server: no |
| Camera | yes | yes | yes (V4L2; clips need ffmpeg) |

Design and verified CLI facts: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Operations: [`docs/RUNBOOK.md`](docs/RUNBOOK.md). Tests: [`docs/TESTING.md`](docs/TESTING.md). Privacy: [`docs/PRIVACY.md`](docs/PRIVACY.md). Security reports: [`SECURITY.md`](SECURITY.md). Contributing: [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Download

Every file is on the [latest release](https://github.com/Publicthrone547/clawdyX/releases/latest) page.

| File | For |
| --- | --- |
| `clawdyX-<version>-darwin-arm64.dmg` | macOS: the desktop app with the bot inside |
| `clawdyx-<version>-darwin-arm64.tar.gz` | macOS: the bot only, for the terminal |
| `clawdyX-<version>-windows-amd64-setup.exe` | Windows: the per-user installer (app and bot) |
| `clawdyX-<version>-windows-amd64.zip` | Windows: the same files without an installer |
| `clawdyx_<version>_amd64.deb` · `clawdyx-<version>-1.x86_64.rpm` | Linux: the package (app and bot) |
| `clawdyX-<version>-linux-amd64.AppImage` · `clawdyX-<version>-linux-amd64.tar.gz` | Linux: the app and bot without a package |
| `clawdyx-<version>-linux-amd64.tar.gz` · `clawdyx-<version>-linux-arm64.tar.gz` | Linux: the bot only, for servers |
| `SHA256SUMS` | the checksums of every file above |

Check a download against `SHA256SUMS`: `shasum -a 256 -c SHA256SUMS --ignore-missing` (macOS), `sha256sum -c SHA256SUMS --ignore-missing` (Linux), or in PowerShell `(Get-FileHash .\clawdyX-<version>-windows-amd64-setup.exe).Hash` and compare it with the line in `SHA256SUMS`.

## Install

You need [Claude Code](https://claude.com/claude-code) installed and signed in (`claude` works in a terminal), and a Telegram bot of your own: create it with @BotFather (`/newbot`) and keep its token ready.

### macOS

1. Download `clawdyX-<version>-darwin-arm64.dmg`, open it and drag clawdyX to Applications.
2. Open clawdyX. Release builds are not notarized yet: if macOS refuses to open it, open **System Settings → Privacy & Security** and click **Open Anyway**.
3. Claude Code: `brew install --cask claude-code`, then run `claude` once in Terminal to sign in.

Screen Recording, Accessibility and Camera grants belong to the app: grant them to **clawdyX** in System Settings when you turn those features on.

### Windows

1. Download `clawdyX-<version>-windows-amd64-setup.exe` and run it. It installs into `%LOCALAPPDATA%\Programs\clawdyX` for your account only, without an administrator prompt. (The `.zip` works too: extract the `clawdyX` folder somewhere permanent and open `clawdyX.exe`.)
2. The files are not code-signed yet, so SmartScreen may warn: click **More info → Run anyway**.
3. Claude Code for Windows: `winget install Anthropic.ClaudeCode`, then run `claude` once in PowerShell to sign in. Claude Code's shell tool needs Git for Windows: `winget install Git.Git`.

The app uses the WebView2 runtime, which Windows 10 and 11 include. Screenshots and Claude's mouse and keyboard tools need no extra permission; the camera needs camera access for desktop apps (Windows Settings → Privacy & security → Camera).

### Linux

1. Download the package for your distribution and install it: `sudo apt install ./clawdyx_<version>_amd64.deb` (Debian, Ubuntu) or `sudo dnf install ./clawdyx-<version>-1.x86_64.rpm` (Fedora). Without a package: the AppImage (it needs `libwebkit2gtk-4.1` installed, and `libfuse2` on Ubuntu 22.04 and later) or the `.tar.gz`.
2. Open clawdyX from the applications menu.
3. Claude Code: `curl -fsSL https://claude.ai/install.sh | bash`, then run `claude` once in a terminal to sign in.

The app needs GTK 3 and WebKitGTK 4.1 (`libgtk-3-0` and `libwebkit2gtk-4.1-0` on Debian and Ubuntu; the packages pull them in). On GNOME the tray icon needs the AppIndicator extension; without it, closing the window quits the app and the bot keeps running. For a server without a desktop, see [Linux server](#linux-server-headless).

## First run

The setup wizard asks for the bot token, finds your Telegram ID from a message you send to the bot, checks Claude Code, lets you pick a project folder, then installs and starts the background worker. Then open Telegram and send `/start` to your bot: any plain message is a prompt for Claude.

The bot runs in that worker, not in the app: closing the window or quitting the app never stops Telegram. The app talks to the worker over a private local socket. Every setting is changed in the app (**Settings**), which also works while the worker is stopped.

Without the app, in a terminal:

```sh
# macOS (zsh), with the worker inside the app
alias clawdyx="/Applications/clawdyX.app/Contents/Helpers/clawdyx"
clawdyx setup && clawdyx service start
```

```powershell
# Windows (PowerShell), with the worker of the installed app
Set-Alias clawdyx "$env:LOCALAPPDATA\Programs\clawdyX\helpers\clawdyx.exe"
clawdyx setup; clawdyx service start
```

```bash
# Linux (bash), with the package installed (clawdyx is on PATH)
clawdyx setup && clawdyx service start
```

`clawdyx service status` shows the worker, `clawdyx check` runs read-only checks, `clawdyx paths` prints where everything is, and `clawdyx help` lists every command.

## Where things live

| What | macOS | Windows | Linux |
| --- | --- | --- | --- |
| Settings | `~/.config/clawdyx/config.json` | `%APPDATA%\clawdyx\config.json` | `$XDG_CONFIG_HOME/clawdyx/config.json` (`~/.config/clawdyx`) |
| Bot token | Keychain, item `clawdyx` | Credential Manager, `clawdyx:…` | keyring item `clawdyx`, or `~/.config/clawdyx/.env` |
| State (sessions, jobs, audit) | `~/Library/Application Support/clawdyx` | `%LOCALAPPDATA%\clawdyx` | `$XDG_STATE_HOME/clawdyx` (`~/.local/state/clawdyx`) |
| Logs | `~/Library/Logs/clawdyx` | `%LOCALAPPDATA%\clawdyx\logs` | `~/.local/state/clawdyx/logs`, and the journal |
| Background worker | LaunchAgent `local.clawdyx` | login entry `local.clawdyx` (HKCU Run) | systemd user unit `clawdyx.service` |

Settings files are private (0600 in a 0700 folder; on Windows, your profile's permissions). The Windows settings of 0.4.x (`%USERPROFILE%\.config\clawdyx`) move to `%APPDATA%\clawdyx` the first time the 1.0 worker is started, the token included.

## Desktop app

The app is the control panel: first-run setup, Start/Stop, sessions, jobs and processes, monitors, settings, diagnostics, Help, and a tray icon (the menu-bar item on macOS).

| Screen | What it does |
| --- | --- |
| Overview | worker, bot, Telegram and Claude status; Start/Stop bot; active session; running jobs |
| Telegram | token (masked), allowed users and roles, detect an ID from a message, connection counters |
| Sessions | new, new and forget, fresh context, switch, rename, archive, restore, forget (optionally delete the Claude Code history), tier, model and effort of the active session |
| Jobs & processes | bot-owned Claude processes with memory and CPU, recent jobs with activity and result, other Claude Code sessions on the computer |
| Monitors | every display, capture one or all, preview, send a capture to Claude, the screen switch, permission help |
| Settings | every option: Claude Code, defaults, workspaces and their command rules, limits, features, retention, logs, folders; reset a section, export and import; About (versions, system, updates, diagnostics); the background worker |
| Diagnostics | health checks with fixes, the redacted log, a sanitized report you preview before saving |
| Help | how everything works, written for the operating system you run |

Closing the window hides it; the tray icon stays. **Quit clawdyX** asks whether to stop the bot too. ⌘1–8 (macOS) or Ctrl+1–8 switch screens, ⌘R or Ctrl+R refreshes, `?` opens Help.

## What it does

| Area | Commands |
| --- | --- |
| Chat | Any plain message is a prompt for the active session. Long jobs stream a preview and can be cancelled with the inline button. |
| Sessions | `/status` `/sessions [all]` `/new [name] [--ws W]` `/newf [name] [--delete]` `/clear` `/switch S2` `/rename S2 name` `/sessions archive S2` `/sessions restore S2` `/forget S2 [--delete]` `/cwd [W]` |
| Claude settings | `/model [name\|alias\|reset] [--default]` `/effort [low\|medium\|high\|xhigh\|max\|auto] [--default]` `/mode observe\|assisted` `/aliases` `/alias set fast haiku` |
| Jobs | `/stop [j-ID\|S-ID\|all]` `/cancel j-ID` `/jobs [n]` `/logs j-ID [lines]` |
| Computer | `/processes` `/usage` `/doctor` `/open youtube.com` (opens a website in the browser of the computer; typed by you, never by Claude) |
| Camera | `/cameras` (list cameras, no capture) · `/camera` (a photo) · `/camera 2` · `/camera video 10` (a 10 s clip, at most 60 s, no sound); opt-in, owner only, see [Camera](#camera-opt-in) |
| Screens | `/screens` (list displays, no capture) · `/screen 2` · `/screen all` (photos sent only to you) · `/look 2 what is wrong here?` (Claude looks at display 2) · `/look all question` · `/screen off` |
| Bot | `/start` (resume) `/pause` `/confirm` `/deny` `/shutdown` (disabled by default) |

`/model fable` sets the **primary** model. Every model change is validated with a one-turn probe before it is saved. The separate `--advisor` model is never touched.

### What each session command touches

| Command | clawdyX session list | Claude Code history on disk |
| --- | --- | --- |
| `/new` | adds a session and makes it active; the old one stays | untouched |
| `/clear` | adds a session with the same workspace, tier, model and effort; the old one stays | untouched (Claude Code's own `/clear` only exists inside a live session) |
| `/newf` | adds a session and forgets the previous active one (restorable for 30 days) | untouched |
| `/newf --delete`, `/forget S2 --delete` | forgets it | deletes exactly that session's transcript file and its folder, after a confirmation that names them |
| `/forget S2` | forgets it (restorable for 30 days) | untouched |
| `/sessions archive S2` | hides it | untouched |
| `/rename S2 name` | changes the label only | untouched |

clawdyX never deletes anything else of Claude Code's. To erase all Claude Code history for a folder, Claude Code itself offers `claude purge <folder>`.

## More than one Telegram user

The owner can allow other numeric Telegram IDs (in the app, or `clawdyx ctl telegram.allowlist.set`). Each entry has a role:

| Role | Can | Cannot |
| --- | --- | --- |
| `owner` (exactly one) | everything | — |
| `user` | chat with Claude, list/create/switch/rename sessions, `/new`, `/newf`, `/clear`, set model and effort for the active session, stop or cancel their own jobs | change tiers or defaults, use screens, the camera or `/open`, forget sessions or delete history, `/doctor`, `/processes`, `/logs`, pause, shut down, stop other people's jobs |

Users share the bot's sessions and workspaces with the owner and get answers in their own chat. Confirmation buttons work only for the person who asked. To find someone's ID, arm detection (app: Telegram → Detect from a message, or `clawdyx ctl telegram.detect.start`), let them message the bot, then read the result; nothing is recorded unless detection is armed, and nobody gets a reply.

## Permission tiers

| Tier | Claude may | Claude may not |
| --- | --- | --- |
| `observe` (default) | Read files in the workspace and run Claude Code's built-in read-only commands | Edit or write files, run other commands, use the web, read outside the workspace |
| `assisted` | The above, plus edit files inside the workspace and run the workspace's allowed commands | Run the commands of the deny list (`rm`, `sudo`, `git push`, downloads, package managers, the system shells of the OS), use the web, read outside the workspace, touch secret paths |
| `automation` (opt-in) | Run shell commands anywhere on the computer, such as opening apps and websites, reading files you name, or scripts, whenever Claude Code's auto-mode safety classifier approves them | The same deny list, plus secret paths. Bypass mode stays off. |

Automation is turned on in the app (**Settings → Features → Automation tier available**) and needs a confirmation button for each session (`/mode automation`). `/mode observe` switches back. The deny list and the secret paths are built in per OS and apply to every workspace in every tier ([`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#21-platform-layer) lists them); a workspace can add more.

Every job also passes `--settings` that disables bypass and auto modes and blocks file-tool reads outside the working directory. Secret locations are denied in every tier: SSH and cloud keys, the token store, browser profiles, and Claude Code's own folder. One consequence is that Claude Code's auto-memory cannot write there; Claude reports those writes as denied and carries on.

### Screens and screen control (opt-in)

With **Settings → Features → Screen features** turned on:

- **`/screens`** lists every connected display (number, name, resolution). It never captures.
- **`/screen N`** sends you a photo of display N (1 is the main display); **`/screen all`** sends one photo per display. Photos go only to the private chat that asked.
- **`/look N question`** captures display N and asks Claude about it in the active session; **`/look all question`** sends one image per display in a single prompt. The image goes to Claude as part of the prompt and is not stored.
- **Automation sessions** also get screen tools from the bot's own MCP server (`clawdyx mcp-screen`): `list_displays` (no capture; it also says whether the mouse and keyboard tools work in this session), `screenshot`, `click`, `move_mouse`, `scroll`, `type_text`, `press_key`, `wait`. Before each screenshot the tool asks the worker whether screen features are still on.
- **`/screen off`** switches every capture path off at once (Telegram, the app, Claude's screen tools, the camera). Only someone at the computer can switch it on again.

Nothing is captured at startup or on a timer. Captures stay in memory for at most five minutes; every capture is recorded in the audit table (display, size, destination; never pixels). Claude Code's auto-mode classifier still reviews each action, and an added system prompt tells Claude to treat on-screen content as untrusted and not to type passwords or payment details, buy, send messages, change settings or approve dialogs unless you asked for that exact action. Clicks are coordinate-based, so expect occasional misses.

- **macOS:** grant Screen Recording (captures) and Accessibility (mouse and keyboard) to clawdyX in System Settings → Privacy & Security; typing also needs Automation permission for System Events. macOS 15 and later ask about once a month whether a program may keep capturing the screen; until someone answers on the Mac, captures fail with a timeout. The grants are tied to the app's signature.
- **Windows:** no permission is needed. Input cannot reach windows that run as administrator, and nothing can be captured while the PC is locked.
- **Linux, X11:** screenshots of every monitor and the mouse and keyboard tools work without extra permissions.
- **Linux, Wayland:** screenshots go through the desktop's screenshot portal (the desktop may ask on the screen) or `grim`; the whole screen is one display. The mouse and keyboard tools are not available on Wayland yet.
- **Linux server:** no graphical session, so screen features are off.

### Camera (opt-in)

Turn it on in the app: **Settings → Features → Camera**, then restart the worker.

- **`/cameras`** lists the cameras (built-in first). It never captures.
- **`/camera`** sends you a photo from the first camera; **`/camera N`** uses camera N. **`/camera video 10`** records a 10-second clip **without sound**. Clips are 1–60 seconds.
- The same rules as for screens: owner only, only to the private chat that asked, in memory for at most five minutes, nothing captured at startup or on a timer, every capture in the audit table. **`/screen off` switches the camera off too.**
- **Claude never gets the camera:** there is no camera tool. See [`docs/PRIVACY.md`](docs/PRIVACY.md#camera) for the one remaining limit in the automation tier.
- **macOS:** macOS asks once, on the Mac, at the first `/camera` (AVFoundation helper inside the app).
- **Windows:** camera access for desktop apps must be on (Windows Settings → Privacy & security → Camera); the app's Media Foundation helper records photos and H.264 clips (Windows N editions need the Media Feature Pack for clips).
- **Linux:** your user needs access to the camera device (the `video` group); photos come straight from V4L2, clips need `ffmpeg`.

**Diagnostics** in the app shows the cameras found and the permission state without capturing.

## Linux server (headless)

The worker runs alone on a server without a desktop, as a systemd user service:

```bash
tar -xzf clawdyx-<version>-linux-amd64.tar.gz && install -m 755 clawdyx-<version>-linux-amd64/clawdyx ~/.local/bin/
printf '%s\n' "$TOKEN" | clawdyx setup -headless -token-stdin -owner-id 123456789 -workspace ~/projects/app
clawdyx service start && loginctl enable-linger "$USER"
```

Without a keyring the token goes into a private file (`~/.config/clawdyx/.env`, mode 0600). `loginctl enable-linger` keeps the worker running from boot, without a login. Logs: `journalctl --user -u clawdyx` and `~/.local/state/clawdyx/logs/bot.log`. Screens are off on a server; the camera works when the server has one.

## Uninstall

Claude Code, its history and your projects are never touched. Revoke the token in @BotFather (`/revoke`) afterwards.

### macOS

In the app: **Settings → Background worker → Uninstall…** (tick the box to delete state and logs), quit clawdyX and move it to the Trash. In Terminal:

```sh
clawdyx service uninstall --purge-state   # the LaunchAgent, state and logs
clawdyx token delete                      # the token in the Keychain
rm -r ~/.config/clawdyx                   # the settings
```

### Windows

Windows Settings → Apps → Installed apps → clawdyX → Uninstall. The uninstaller removes the worker's login entry and offers to delete the settings, the state and the token. From the `.zip`, in PowerShell:

```powershell
clawdyx service uninstall -purge-state     # the login entry, state and logs
clawdyx token delete                       # the token in the Credential Manager
Remove-Item -Recurse "$env:APPDATA\clawdyx"   # the settings
```

then delete the `clawdyX` folder.

### Linux

```bash
clawdyx service uninstall --purge-state    # the systemd unit, state and logs
clawdyx token delete                       # the token in the keyring, or its file
rm -r ~/.config/clawdyx                    # the settings
sudo apt remove clawdyx                    # or: sudo dnf remove clawdyx, or delete the AppImage
```

## Build from source

Everything builds from a checkout with Go (the version in `go.mod`) and Node.js 22 or newer; `make help` lists every target, `make check-all` runs every test and the type checks for all three systems.

### macOS

Xcode command-line tools. `make app` builds `bin/clawdyX.app` (app, worker, capture and camera helpers); `make dmg` the disk image; `make setup` and `make start` set up and run the worker from source.

### Windows

Git for Windows (its bash runs the Makefile), GNU make, and for the camera helper the Visual Studio Build Tools with the C++ workload. `make build-windows` writes `bin/windows/clawdyX/`; `make installer-windows` the installer (NSIS). The worker and the app also cross-compile from any system (`CGO_ENABLED=0`).

### Linux

The app needs cgo and the GTK 3 and WebKitGTK 4.1 development packages (see `desktop/build/linux/README.md`). `make app-linux` builds `bin/linux/clawdyX/`, `make package-linux` the `.deb`, `.rpm` and `.tar.gz`; `make build-linux` cross-compiles the worker for amd64 and arm64 from any system.

## Layout

```
cmd/clawdyx          executable: run, setup, check, service, ctl, paths, token, backup
cmd/clawdyxw         Windows login launcher (no console window)
internal/platform    operating system facts and words; the only place that branches on the OS
internal/config      fail-closed configuration, per-OS folders, migration
internal/settings    every setting: info, apply, reset, export, import
internal/telegram    gate, dispatcher, commands, streaming, sender/outbox
internal/agent       claude CLI adapter (argv/env builder, process groups, stream-json parser)
internal/jobs        queues, concurrency cap, cancellation, timeouts, recovery
internal/sessions    logical sessions and defaults
internal/policy      tiers → CLI permission flags, per-OS deny lists and secret paths
internal/processes   process sampling per OS, cached `claude agents --json`
internal/store       SQLite state (modernc, no CGO)
internal/render      Markdown → escaped Telegram HTML, chunking
internal/screen      displays, capture broker, MCP screen tools; macOS, Windows, X11, Wayland
internal/camera      cameras, photo/clip broker; AVFoundation, Media Foundation, V4L2
internal/vault       the system token store
internal/observability  redaction, logging, counters
desktop              the Wails app; desktop/host holds its per-OS code
```

## Repository

The public history of this repository starts at version 0.2.0. The owner pushes manually; nothing is pushed by tooling. Secrets, local config, state databases, logs, build output and private planning notes are git-ignored (see `.gitignore`). `make check-secrets` scans tracked files for token-shaped strings.

## License

clawdyX is **source-available** under the [PolyForm Noncommercial License 1.0.0](LICENSE) (official text: https://polyformproject.org/licenses/noncommercial/1.0.0). In short, and without this being legal advice: the license permits use for noncommercial purposes and does not grant permission for commercial use. Anyone who wants to use clawdyX commercially needs separate permission from the rightsholder. The exact terms, including what counts as noncommercial, are those of the license text itself; ask a lawyer if you are unsure. This is not an OSI-approved open-source license.

The license covers the original material in this repository only. Third-party dependencies keep their own licenses (see [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md), generated by `scripts/gen-notices.sh`). Claude Code is a separate product with its own terms; clawdyX only drives the copy you installed and logged into yourself.
