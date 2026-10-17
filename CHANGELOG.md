# Changelog

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
