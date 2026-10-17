# Privacy and data handling

clawdyX runs entirely on your Mac. It has no server, no telemetry, no analytics, no crash upload and no update check. It talks to exactly two things: Telegram's Bot API (`api.telegram.org`), to receive your messages and send replies, and the Claude Code CLI installed on your Mac, which then talks to Anthropic the same way it does when you use it in Terminal.

## What is stored where

| Data | Where | Protection | Notes |
| --- | --- | --- | --- |
| Telegram bot token | macOS login Keychain, item `clawdyx` (created through Apple's `security` tool; `-token-file` keeps it in `~/.config/clawdyx/.env` instead) | Keychain encryption; readable by programs running as your macOS user | never logged; redacted from every message and diagnostic; shown only as its last four characters |
| Allowed Telegram IDs with labels and roles, workspaces, settings | `~/.config/clawdyx/config.json` | 0600 | never sent anywhere |
| Sessions, job history, aliases, audit log | `~/Library/Application Support/clawdyx/state.db` | 0600 SQLite, folder 0700 | stores a hash of each prompt, an optional short preview, a ≤ 2 KB preview of the reply, token counts and timings; never full prompts, full replies or secrets |
| Logs | `~/Library/Logs/clawdyx/` | 0600, rotated at 20 MB × 5 | command categories, timings, states, redacted errors; no message bodies |
| Backups | `~/Library/Application Support/clawdyx/backups/` | 0600 | copies of `state.db` only |
| Claude transcripts | `~/.claude/projects/…` | owned by Claude Code | Claude Code keeps these under its own retention (`cleanupPeriodDays`, 30 days by default). clawdyX sets `CLAUDE_CODE_SKIP_PROMPT_HISTORY=1` for its jobs so prompts are not also added to Claude Code's global prompt history |
| Screenshots | memory only | — | see below |

## What leaves the Mac

- **To Telegram:** the replies the bot sends to the allowed users' private chats, including Claude's answers, job status, and screenshots that the owner explicitly requested with a command. Telegram stores chat history on its servers under its own terms.
- **To Anthropic, through Claude Code:** your prompts, the files and command output Claude reads while working in the selected workspace, and screenshots only when a screen tool was used in an automation session or you asked Claude to look at the screen. This is the normal Claude Code request flow under your own account and its data settings.
- **Nothing else.** clawdyX never uploads files, logs or diagnostics anywhere.

## Screen capture

- Disabled by default (`"desktop": {"screen_control": false}`).
- When enabled, a capture happens only when the owner types `/screen` or `/look`, clicks Capture in the desktop app, or when Claude uses the screenshot tool inside an automation-tier session that the owner switched on with a confirmation button. `/screens` only lists displays. Nothing is captured at startup or on a timer.
- Each display is captured separately and never stitched. Images are kept in memory for at most five minutes (at most six at a time) and passed to Telegram or Claude directly. macOS's `screencapture` can only write files, so each capture passes through a private temporary folder (0700) and is deleted before the image is used. Images are re-encoded, which drops all metadata.
- Where each capture went is recorded in the audit table (display, size, destination, who asked); pixels are never stored.
- `/screen off` (or the app's switch) disables every capture path, including Claude's screenshot tool in a running job; only someone at the Mac can switch it on again.
- macOS asks for Screen Recording permission and, on macOS 15 and later, asks again about once a month. clawdyX cannot grant itself these permissions.

## Deleting your data

- `clawdyx service uninstall --purge-state` stops the service and deletes the state folder and logs.
- Delete `~/.config/clawdyx/` to remove the token and settings, then revoke the token in @BotFather.
- One session's Claude Code history: `/forget S2 --delete` (or Delete history in the app) removes exactly that session's transcript file and folder after a confirmation that lists them. Claude Code documents that these files may be deleted by hand; clawdyX touches nothing else under `~/.claude`.
- All history of a project folder: `claude purge <folder>` (Claude Code's own command).
