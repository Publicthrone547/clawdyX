# Changelog

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
