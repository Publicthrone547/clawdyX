# Privacy and data handling

clawdyX runs entirely on your Mac. It has no server, no telemetry, no analytics, no crash upload and no update check. It talks to exactly two things: Telegram's Bot API (`api.telegram.org`), to receive your messages and send replies, and the Claude Code CLI installed on your Mac, which then talks to Anthropic the same way it does when you use it in Terminal.

## What is stored where

| Data | Where | Protection | Notes |
| --- | --- | --- | --- |
| Telegram bot token | `~/.config/clawdyx/.env` | file mode 0600 in a 0700 folder | never logged; redacted from every message and diagnostic |
| Your Telegram user ID, workspaces, settings | `~/.config/clawdyx/config.json` | 0600 | never sent anywhere |
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
- When enabled, a capture happens only when the owner types a screen command, or when Claude uses the screenshot tool inside an automation-tier session that the owner switched on with a confirmation button. Nothing is captured at startup or on a timer.
- Images are kept in memory and passed to Telegram or Claude directly. Temporary files used during capture live in a private temporary folder and are deleted immediately.
- macOS asks for Screen Recording permission and, on macOS 15 and later, asks again about once a month. clawdyX cannot grant itself these permissions.

## Deleting your data

- `clawdyx service uninstall --purge-state` stops the service and deletes the state folder and logs.
- Delete `~/.config/clawdyx/` to remove the token and settings, then revoke the token in @BotFather.
- Claude Code's own transcripts are managed by Claude Code (`claude purge <folder>` erases a project's history; see its documentation).
