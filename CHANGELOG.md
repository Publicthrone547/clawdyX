# Changelog

## 1.0.1 — 2026-10-19

The first published 1.x release, for **macOS and Windows**; 1.0.0 was tagged but never released. Same features as 1.0.0 below; Linux support is in the code but not released yet.

- **Build.** All release files are built on the Mac (`make release`): the macOS app and worker, and the Windows installer and zip (cross-compiled; the Windows camera helper is built on Windows and copied in). The GitHub Actions workflows were removed. The macOS camera helper also builds with the macOS 14 SDK.

## 1.0.0 — 2026-10-19

clawdyX now runs on macOS, Windows and Linux with the same features, and every setting is edited in the app.

- **Windows.** The bot and the desktop app are supported, not a preview: a per-user installer (`clawdyX-1.0.0-windows-amd64-setup.exe`, no administrator rights) next to the portable zip; the background worker starts at login without a console window and restarts itself after a crash; screenshots, `/screen`, `/look` and Claude's screen tools (mouse and keyboard) work on every monitor and scaling; `/cameras`, `/camera` and `/camera video` use the Media Foundation camera helper; Claude Code's Git Bash location is a setting; the Windows deny list blocks PowerShell (Claude Code's PowerShell tool included), cmd, download and system tools in every tier. Settings moved to `%APPDATA%\clawdyx` (moved automatically when the 1.0 worker starts, the token included).
- **Linux.** New: the worker and the desktop app (`.deb`, `.rpm`, AppImage, tar.gz; amd64, and arm64 for the worker). The worker runs as a systemd user service, also on headless servers (`clawdyx setup -headless`, linger); the token goes to the system keyring, or to a private file when no keyring exists; screens and Claude's screen tools work on X11; on Wayland screenshots go through the desktop portal and input tools are not available yet; `/camera` uses V4L2, clips need `ffmpeg`. Linux has been built and tested in CI, not yet on a Linux desktop.
- **Settings.** Every option is in the app, including Claude Code's location and setting sources, budgets, queue and output limits, the child PATH, workspace allowed/denied command lists, log and state folders and retention; reset per section or everything; export and import. The app can also edit settings while the worker is down. Nothing tells you to edit `config.json` any more.
- **Help.** A Help screen in the app, written for the operating system you run; the bot's `/help`, `/doctor` and error replies likewise. Shortcuts are ⌘ on macOS and Ctrl elsewhere; `?` opens Help.
- **About** shows the operating system, architecture, session, the worker service, the folders, the worker and Claude Code versions, and copies a redacted diagnostics report. The update check offers the right file for your system. Optional notifications when a job ends while the window is hidden.
- **Security.** Built-in per-OS deny lists and secret paths apply to every workspace in every tier; existing settings keep working. Claude still never gets the camera; `/screen off` still switches every capture path off. `clawdyx token delete` removes a configuration's token.
- **Build and release.** CI on macOS, Windows and Linux; a release workflow that builds every file on its own system with `SHA256SUMS` into a draft release; one version source (the Makefile); `THIRD_PARTY_NOTICES.md` covers all targets; winget and Scoop manifests; `CONTRIBUTING.md`.
- **Compatibility.** Settings and the state database of 0.4.x are read as they are; no database migration. macOS behaviour is unchanged.

## 0.4.1 — 2026-10-18

- **Camera in Settings.** Turn the camera on or off in the app: **Settings → Features → Camera** (then restart the worker). Editing config.json is no longer needed. The bot's "camera is off" reply points there.
- **Diagnostics shows the camera:** off, or the cameras found and whether macOS granted the Camera permission, with a button to the Camera page of System Settings. The check never takes a photo.
- **Fix:** Settings → Logs → "Show in Finder" works again (broken in 0.4.0). A test now checks that every method the app's UI calls exists in the app.

## 0.4.0 — 2026-10-18

- **Windows (preview).** clawdyX now builds for Windows 10/11 x64: the bot (`clawdyx.exe`) and the desktop app (`clawdyX.exe`), cross-compiled from a Mac with `make build-windows`. The Telegram bot, sessions, jobs, models, tiers, `/open` (default browser), `/processes` and `/doctor` work; the token goes into the Windows Credential Manager; `clawdyx service start` runs the worker in the background and at every login without administrator rights. Screens and the camera reply that they are macOS only. **Not yet tested on a real Windows PC.** Download: `clawdyX-0.4.0-windows-amd64.zip`. Details: [Windows in the README](https://github.com/Publicthrone547/clawdyX/blob/v0.4.0/README.md#windows-preview) and [the runbook](https://github.com/Publicthrone547/clawdyX/blob/v0.4.0/docs/RUNBOOK.md#windows).
- **Camera (opt-in, macOS).** With `"desktop": {"camera": true}`: `/cameras` lists the cameras, `/camera [N]` sends you a photo, `/camera video SECONDS [N]` a clip of 1–60 seconds without sound. Owner only, sent only to the private chat that asked, kept in memory for at most five minutes, every capture in the audit table, and `/screen off` switches the camera off too. Claude gets no camera access. macOS asks for the Camera permission on the Mac at the first `/camera`; a missing permission is named in the reply. See [Camera](https://github.com/Publicthrone547/clawdyX/blob/v0.4.0/README.md#camera-opt-in) and [PRIVACY](https://github.com/Publicthrone547/clawdyX/blob/v0.4.0/docs/PRIVACY.md#camera).
- **Screens:** `/look` drops its captures as soon as the job is queued instead of after the reply.
- **Worker:** new control method `worker.stop` (clean exit; `clawdyx ctl worker.stop`). `clawdyx ctl help` lists the settings and worker methods.
- **State database v4** (camera audit columns). A backup is written to `backups/` before the upgrade; 0.3.x cannot open a v4 database, so restore that backup to downgrade.
- **Build:** `make release` builds every release file with checksums; `make check-all` also type-checks the Windows build; `THIRD_PARTY_NOTICES.md` covers the Windows-only modules.

## 0.3.3 — 2026-10-17

- **Claude can look at every display.** `/look all question` sends a screenshot of each display in one prompt; `/look question` still uses the main display and `/look N` one display.
- **Screen tools know about all displays.** Automation sessions get `list_displays` (captures nothing) and are told to list the displays and take one screenshot per display when the request is not about a specific one. Before, Claude rarely looked beyond the main display because it did not know the others existed.
- **README:** a download button, release badges and a Download section.

## 0.3.2 — 2026-10-17

- **Desktop app shows its version.** The sidebar shows `v<version>` (click it for details). **Settings → About clawdyX** lists the app, worker and Claude Code versions, warns when the worker still runs an older version after an update, and links to this version's release notes and all releases. **Check for updates** asks GitHub for the latest release, only when clicked.

## 0.3.1 — 2026-10-17

- **Fix: sessions could not be continued.** 0.3.0 started Claude with `CLAUDE_CODE_SKIP_PROMPT_HISTORY=1`. On Claude Code 2.1.292 that variable also stops transcripts from being written, so every follow-up message in a session failed with `error_during_execution`. clawdyX no longer sets it. Sessions started on 0.3.0 have no transcript and cannot be resumed; start them again with `/new`.
- **Fix: a missing transcript is reported clearly.** Claude Code 2.1.292 reports it as an error result. The bot now recognises that, marks the session as no longer resumable and suggests `/new`, instead of showing `error_during_execution /`.
- **Privacy docs:** prompts sent through clawdyX are added to Claude Code's global prompt history (`~/.claude/history.jsonl`), like any Claude Code use. `--delete` removes the session's transcript but not those entries.

## 0.3.0 — 2026-10-17

- **Desktop app (clawdyX.app):** Wails v3 + Vue 3 + Tailwind 4 control panel with a first-run wizard, Overview (Start/Stop), Telegram (token, allowed users, detection), Sessions, Jobs & processes, Monitors, Settings and Diagnostics; menu-bar item; close hides the window; `make app`, `make dmg`.
- **Worker control socket:** JSON-RPC over a 0600 Unix socket with a peer-uid check; `clawdyx ctl`. Telegram polling can be started and stopped without stopping the worker.
- **Screens:** all displays, `/screens`, `/screen N|all|off`, `/look N question`; captures go through one broker (switch, in-memory only, audit); a bundled ScreenCaptureKit helper captures by display ID and checks the permission without capturing.
- **Sessions:** `/newf [--delete]`, `/clear`, `/forget [--delete]`, `/rename`, `/sessions restore`; deleting history removes exactly that session's documented transcript files.
- **Security:** bot token in the macOS Keychain (`setup -migrate-token`); allowed users with owner/user roles; confirmations bound to the person who asked; replies go to the asking chat.
- **Docs:** ARCHITECTURE, PRIVACY, SECURITY, RELEASE (measured baseline), THIRD_PARTY_NOTICES.

## 0.2.0 — 2026-10-17

- Project renamed to clawdyX: binary `clawdyx`, service `local.clawdyx`, config in `~/.config/clawdyx`, state in `~/Library/Application Support/clawdyx`, logs in `~/Library/Logs/clawdyx`, env override `CLAWDYX_CONFIG`.
- `clawdyx setup` (`make setup`): guided first-time setup with hidden token entry, Telegram ID detection from a message you send to the bot, workspace and browser detection, private 0600 files and the read-only checks.
- `clawdyx service start|stop|restart|status` and the matching `make` targets replace manual `launchctl` commands.
- The owner's Telegram ID is configuration (`telegram.owner_user_id`, fail-closed when missing) instead of a compiled constant. Example config and docs contain placeholders only.
- `LICENSE`: PolyForm Noncommercial License 1.0.0 (verbatim official text).
- Public history starts here. The earlier private development history was kept offline by the owner and never published.

## 0.1.0 — 2026-10-16 (private development)

- Telegram front end with the owner-only gate, sessions, jobs, streaming, model and effort controls, aliases and confirmations.
- Permission tiers observe / assisted / opt-in automation, owner-typed `/open`, opt-in screen control through an MCP server (`mcp-screen`), `/screenshot`.
- SQLite state, single-instance lock, restart recovery, LaunchAgent service, redacted logging, test suite with a fake `claude` and a fake Bot API.
