# Privacy and data handling

clawdyX runs entirely on your computer. It has no server, no telemetry, no analytics, no crash upload and no automatic update check. It talks to exactly two things: Telegram's Bot API (`api.telegram.org`), to receive your messages and send replies, and the Claude Code CLI installed on your computer, which then talks to Anthropic the same way it does when you use it yourself. This is the same on macOS, Windows and Linux.

The one exception is a button: **Settings → About clawdyX → Check for updates** in the desktop app asks GitHub (`api.github.com`) for the latest clawdyX release, only when you click it. The request carries no account, token or Telegram data; GitHub sees your IP address and the app version in the User-Agent. The download it offers is opened in your browser; clawdyX downloads nothing itself.

## What is stored where

| Data | macOS | Windows | Linux | Notes |
| --- | --- | --- | --- | --- |
| Telegram bot token | login Keychain, item `clawdyx` | Credential Manager, `clawdyx:telegram-bot-token@…` | Secret Service keyring (GNOME Keyring, KWallet), item `clawdyx` | readable by programs running as your user; never logged; redacted from every message and diagnostic; shown only as its last four characters. Without a token store (a Linux server), or with `setup -token-file`, a private file `.env` (0600) next to the settings |
| Allowed Telegram IDs with labels and roles, workspaces, settings | `~/.config/clawdyx/config.json` | `%APPDATA%\clawdyx\config.json` | `~/.config/clawdyx/config.json` | 0600 in a 0700 folder (Windows: your profile's permissions); never sent anywhere |
| Sessions, job history, aliases, audit log | `~/Library/Application Support/clawdyx/state.db` | `%LOCALAPPDATA%\clawdyx\state.db` | `~/.local/state/clawdyx/state.db` | 0600 SQLite; a hash of each prompt, an optional short preview, a ≤ 2 KB preview of the reply, token counts and timings; never full prompts, full replies or secrets |
| Logs | `~/Library/Logs/clawdyx/` | `%LOCALAPPDATA%\clawdyx\logs\` | `~/.local/state/clawdyx/logs/` and the user journal | 0600, rotated at 20 MB × 5; command categories, timings, states, redacted errors; no message bodies |
| Backups | `<state>/backups/` | `<state>\backups\` | `<state>/backups/` | copies of `state.db` only |
| Claude transcripts | `~/.claude/projects/…` | `%USERPROFILE%\.claude\projects\…` | `~/.claude/projects/…` | owned by Claude Code under its own retention (`cleanupPeriodDays`, 30 days by default); they make a session resumable. Prompts are also added to Claude Code's global prompt history (`.claude/history.jsonl`) |
| Screenshots, camera photos and clips | memory only | memory only | memory only | at most five minutes; see below for the short-lived files on the way |

## The desktop app

The app stores only per-window conveniences (appearance, last screen, the Help topic, whether to show notifications) in its own web storage. It reads status from the worker over a local socket that only your user can open, and shows the bot token only as its last four characters. A diagnostic report is shown in full before it is saved or copied; tokens are removed, home paths shortened, Telegram IDs left out unless you tick the box, and screenshots are never included. Captures appear in the app only after you click Capture; they stay in memory. Notifications, when you turn them on, name the job and how it ended, never the prompt or the reply.

## What leaves the computer

- **To Telegram:** the replies the bot sends to the allowed users' private chats, including Claude's answers, job status, and screenshots, camera photos and clips that the owner explicitly requested with a command. Telegram stores chat history on its servers under its own terms.
- **To Anthropic, through Claude Code:** your prompts, the files and command output Claude reads while working in the selected workspace, and screenshots only when a screen tool was used in an automation session or you asked Claude to look at the screen. This is the normal Claude Code request flow under your own account and its data settings.
- **Nothing else.** clawdyX never uploads files, logs or diagnostics anywhere.

## Screen capture

- Disabled by default (Settings → Features → Screen features).
- When enabled, a capture happens only when the owner types `/screen` or `/look`, clicks Capture in the desktop app, or when Claude uses the screenshot tool inside an automation-tier session that the owner switched on with a confirmation button. `/screens` only lists displays. Nothing is captured at startup or on a timer.
- Each display is captured separately and never stitched. Images are kept in memory for at most five minutes (at most six at a time) and passed to Telegram or Claude directly. Images are re-encoded, which drops all metadata.
- Where each capture went is recorded in the audit table (display, size, destination, who asked); pixels are never stored.
- `/screen off` (or the app's switch) disables every capture path, including Claude's screenshot tool in a running job; only someone at the computer can switch it on again.

### Per operating system

| | How a capture is made | Files on the way | Permission |
| --- | --- | --- | --- |
| macOS | `screencapture` (the app's helper) | a private temporary folder (0700), deleted before the image is used | Screen Recording; macOS 15 and later ask again about once a month |
| Windows | GDI, in the worker's memory | none | none; nothing can be captured while the PC is locked |
| Linux, X11 | the X server, in the worker's memory | none | none |
| Linux, Wayland | the desktop's screenshot portal, or `grim` (to memory) | the portal writes a file where the desktop chooses; clawdyX reads it and deletes it at once | the desktop may ask on the screen; refusing it refuses the capture |
| Linux server | — | — | screen features are off without a graphical session |

clawdyX cannot grant itself these permissions.

## Camera

- Disabled by default (Settings → Features → Camera).
- A capture happens only when the owner types `/camera` (one photo) or `/camera video SECONDS` (a clip of 1–60 s, **without sound**) in their private chat. `/cameras` only lists cameras. Nothing is captured at startup or on a timer. Allowed users with the `user` role cannot use the camera.
- The photo or clip goes only to the private chat that asked. It is kept in memory for at most five minutes (in practice it is dropped as soon as it was sent).
- Every capture is recorded in the audit table: kind (photo or video), camera, clip length, size, destination and who asked; pixels are never stored.
- `/screen off` (or the app's switch) turns the camera off too; only someone at the computer can switch it on again.

| | How a capture is made | Files on the way | Permission |
| --- | --- | --- | --- |
| macOS | the app's AVFoundation helper | a private temporary folder (0700), deleted before the media is used | macOS asks once, on the Mac, at the first `/camera`; the green light is on while it records |
| Windows | the app's Media Foundation helper (`clawdyx-camera.exe`) | a private folder under `%LOCALAPPDATA%\clawdyx\tmp` (inside your profile, not the shared temp folder), deleted before the media is used | camera access for desktop apps (Windows Settings → Privacy & security → Camera); Windows shows that the camera is in use |
| Linux | V4L2 in the worker (photos, in memory); `ffmpeg` for clips | clips: a private folder (0700) under `~/.local/state/clawdyx/tmp`, deleted before the clip is used | access to `/dev/video*` (the `video` group) |

**Claude gets no camera access.** There is no camera tool for Claude and no control-socket method for the camera; the macOS and Windows helpers refuse to capture unless the clawdyX worker itself started them. One limit remains on every OS: in the opt-in automation tier Claude can run shell commands as your user, and a program written for that purpose could use the camera the way any program of yours can (on macOS through the permissions the app passes to the programs it starts, on Windows through the camera API when camera access for desktop apps is on, on Linux through `/dev/video0` when your user is in the `video` group). Leave the camera off (or do not use automation) if that matters to you. The same applies to the screen.

## Deleting your data

- `clawdyx service uninstall --purge-state` stops the service and deletes the state folder and logs. The Windows uninstaller offers the same, plus the settings and the token.
- `clawdyx token delete` removes the bot token from the token store (and its file, if any); delete the settings folder to remove the settings; then revoke the token in @BotFather.
- One session's Claude Code history: `/forget S2 --delete` (or Delete history in the app) removes exactly that session's transcript file and folder after a confirmation that lists them. clawdyX touches nothing else of Claude Code's, so the session's prompts remain in Claude Code's global prompt history.
- All history of a project folder: `claude purge <folder>` (Claude Code's own command).
