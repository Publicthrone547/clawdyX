# Data and privacy

clawdyX runs entirely on {{.W.ThisComputer}}. It has no server, no telemetry, no analytics and no crash upload. It talks to two things: Telegram's Bot API, to receive your messages and send replies, and the Claude Code on {{.W.ThisComputer}}, which talks to Anthropic as it does when you use it yourself.

The one exception is a button: **Settings → About clawdyX → Check for updates** asks GitHub for the latest release, only when you click it.

## What is stored where

| Data | Where | Notes |
| --- | --- | --- |
| Bot token | {{.W.TokenStore}}{{if .Linux}} (or a private file when there is no keyring){{end}} | never logged; shown only as its last four characters |
| Allowed users, workspaces, settings | `{{.Paths.Config}}` | {{if .Windows}}inside your user profile{{else}}mode 0600{{end}} |
| Sessions, job history, audit log | `{{.Paths.State}}` | a hash and a short preview of each prompt, a preview of each reply; never full prompts or replies |
| Logs | `{{.Paths.Logs}}` | categories, timings and redacted errors; no message text |
| Claude Code transcripts | Claude Code's own folder | kept by Claude Code under its own retention |
| Screenshots, photos, clips | memory only, at most five minutes | {{if .Darwin}}they pass through a private temporary folder that is deleted at once{{else if .Windows}}the camera helper writes into a private folder under `{{.Paths.State}}\tmp` that is deleted at once{{else}}clips pass through a private folder under `{{.Paths.State}}/tmp`, and Wayland screenshots through the file the desktop writes; both are deleted at once{{end}} |

## What leaves {{.W.ThisComputer}}

- **To Telegram:** the replies the bot sends to the allowed users' private chats, including screenshots, photos and clips the owner asked for.
- **To Anthropic, through Claude Code:** your prompts, the files and command output Claude reads in the workspace, and screenshots only when Claude used a screen tool or you asked it to look.
- **Nothing else.** clawdyX never uploads files, logs or diagnostics. A diagnostic report is shown to you in full before you save it, with tokens removed.

## Deleting your data

See [Uninstall](help:uninstall). One session's Claude Code history: `/forget S2 --delete`, or **Sessions → Forget… → Also delete its Claude Code history**.
