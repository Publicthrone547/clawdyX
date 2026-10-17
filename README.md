# clawdyX — Telegram control plane for local Claude Code

A private, single-operator Telegram bot that drives the **Claude Code CLI already installed and logged in on this Mac**. It runs `claude -p` as a child process per message (stream-json output, `--resume` for continuity), so it reuses your existing claude.ai login. It never uses an API key and never bypasses Claude Code's permission system.

Only the Telegram user whose numeric ID you enter during setup is served, and only in a private chat. Every other update is dropped silently before any lookup or side effect.

Design and verified CLI facts: [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md). Operations: [`docs/RUNBOOK.md`](docs/RUNBOOK.md). Tests: [`docs/TESTING.md`](docs/TESTING.md). Privacy: [`docs/PRIVACY.md`](docs/PRIVACY.md). Security reports: [`SECURITY.md`](SECURITY.md).

## Quick start

Requirements: macOS (tested on Apple silicon), Go 1.27 or newer, and Claude Code installed and logged in (`claude` works in your terminal). Create a bot with @BotFather in Telegram and keep its token ready.

```sh
make setup      # asks for the token, your Telegram ID and a project folder; writes ~/.config/clawdyx (private); runs the checks
make start      # runs the bot in the background now and at every login
```

Then open Telegram and send `/start` to your bot.

| Command | What it does |
| --- | --- |
| `make run` | run in the foreground instead (Ctrl-C stops it) |
| `make stop` · `make restart` · `make status` | control the background service |
| `make logs` | follow the log |
| `make doctor` | read-only checks: config, token, Claude CLI, login, Telegram |
| `make help` | list these commands |

The wizard can detect your Telegram ID: leave the ID empty, send any message to your bot, press Enter. Everything it writes stays on this Mac in `~/.config/clawdyx/` (mode 0600). Manual setup without the wizard is described in [`docs/RUNBOOK.md`](docs/RUNBOOK.md).

## What it does

| Area | Commands |
| --- | --- |
| Chat | Any plain message is a prompt for the active session. Long jobs stream a preview and can be cancelled with the inline button. |
| Sessions | `/status` `/sessions [all]` `/new [name] [--ws W]` `/switch S2` `/sessions archive S2` `/cwd [W]` |
| Claude settings | `/model [name\|alias\|reset] [--default]` `/effort [low\|medium\|high\|xhigh\|max\|auto] [--default]` `/mode observe\|assisted` `/aliases` `/alias set fast haiku` |
| Jobs | `/stop [j-ID\|S-ID\|all]` `/cancel j-ID` `/jobs [n]` `/logs j-ID [lines]` |
| Machine | `/processes` `/usage` `/doctor` `/open youtube.com` (opens a website in the Mac's browser; typed by you, never by Claude) |
| Screens | `/screens` (list displays, no capture) · `/screen 2` · `/screen all` (photos sent only to you) · `/look 2 what is wrong here?` (Claude looks at display 2) · `/screen off` |
| Bot | `/start` (resume) `/pause` `/confirm` `/deny` `/shutdown` (disabled by default) |

`/model fable` sets the **primary** model (`fable` resolves to `claude-fable-5-1` on CLI 2.1.292). Every model change is validated with a one-turn probe before it is saved. The separate `--advisor` model is never touched.

## Permission tiers

| Tier | Claude may | Claude may not |
| --- | --- | --- |
| `observe` (default) | Read files in the workspace and run Claude Code's built-in read-only commands | Edit or write files, run other commands, use the web, read outside the workspace |
| `assisted` | The above, plus edit files inside the workspace and run the workspace `allowed_commands` | Run `rm`, `sudo`, `git push`, `git remote`, `curl`, `wget`, `brew`, `claude`, use the web, read outside the workspace, touch secret paths |
| `automation` (opt-in) | Run shell commands anywhere on the Mac, such as opening apps and websites, reading files you name, scripts, or `osascript`, whenever Claude Code's auto-mode safety classifier approves them | The same deny list as `assisted`, plus secret paths. Bypass mode stays off. |

Automation requires `"modes": {"automation_enabled": true}` in config.json and a confirmation button for each session (`/mode automation`). `/mode observe` switches back.

### Screens and screen control (opt-in)

With `"desktop": {"screen_control": true}` in config.json:

- **`/screens`** lists every connected display (number, name, resolution). It never captures.
- **`/screen N`** sends you a photo of display N (1 is the main display); **`/screen all`** sends one photo per display. Photos go only to the private chat that asked.
- **`/look N question`** captures display N and asks Claude about it in the active session. The image goes to Claude as part of the prompt (in memory, through Claude Code's normal request flow) and is not stored.
- **Automation sessions** also get screen tools from the bot's own MCP server (`clawdyx mcp-screen`): `screenshot` (optional `display` number), `click`, `move_mouse`, `scroll`, `type_text`, `press_key`, `wait`. Before each screenshot the tool asks the worker whether screen features are still on.
- **`/screen off`** switches every capture path off at once (Telegram, the desktop app, Claude's screenshot tool). It can be switched on again only from the Mac: the clawdyX app, or `clawdyx ctl screens.enabled.set '{"enabled":true}'`.

Nothing is captured at startup or on a timer. Captures stay in memory for at most five minutes; every capture is recorded in the audit table (display, size, destination; never pixels).

- **Safety:** Claude Code's auto-mode classifier still reviews each action. An added system prompt tells Claude to treat on-screen content as untrusted. It must not type passwords or payment details, buy, send messages, change settings or approve dialogs unless you asked for that exact action.
- **macOS permissions:** grant Screen Recording (captures) and Accessibility (mouse/keyboard) in System Settings → Privacy & Security to the program that runs the bot: your terminal app when run by hand, or the `clawdyx` binary under launchd. Typing also needs Automation permission for System Events. `/doctor` and `clawdyx ctl diagnostics.run` report what the worker can do; Screen Recording is reported from the last real capture, because macOS offers no reliable way to check it without capturing.
- **Monthly reminder:** macOS 15 and later ask about once a month whether a program may keep capturing the screen without the system picker. The dialog appears on the Mac; until someone answers it there, captures from Telegram fail with a timeout.
- **Rebuilds:** macOS ties these grants to the binary's signature, so rebuilding `clawdyx` may require granting them again.
- **Limits:** clicks are coordinate-based, so expect occasional misses.

Every job also passes `--settings` that disables bypass and auto modes and blocks file-tool reads outside the working directory. Secret locations are denied in every tier: `~/.ssh`, `~/.aws`, `~/.config`, `~/.claude`, keychains, and browser profiles. One consequence is that Claude Code's auto-memory cannot write under `~/.claude`. Claude reports those writes as denied and carries on.

## Layout

```
cmd/clawdyx        executable: run, check, service, backup
internal/config      fail-closed configuration
internal/telegram    gate, dispatcher, commands, streaming, sender/outbox
internal/agent       claude CLI adapter (argv/env builder, process groups, stream-json parser)
internal/jobs        queues, concurrency cap, cancellation, timeouts, recovery
internal/sessions    logical sessions and defaults
internal/policy      tiers → CLI permission flags, path checks
internal/processes   ps/sysctl sampling, cached `claude agents --json`
internal/store       SQLite state (modernc, no CGO)
internal/render      Markdown → escaped Telegram HTML, chunking
internal/observability  redaction, logging, counters
```

## Repository

The public history of this repository starts at version 0.2.0. The owner pushes manually; nothing is pushed by tooling. Secrets, local config, state databases, logs, build output and private planning notes are git-ignored (see `.gitignore`). `make check-secrets` scans tracked files for token-shaped strings.

## License

clawdyX is **source-available** under the [PolyForm Noncommercial License 1.0.0](LICENSE) (official text: https://polyformproject.org/licenses/noncommercial/1.0.0). In short, and without this being legal advice: the license permits use for noncommercial purposes and does not grant permission for commercial use. Anyone who wants to use clawdyX commercially needs separate permission from the rightsholder. The exact terms, including what counts as noncommercial, are those of the license text itself; ask a lawyer if you are unsure. This is not an OSI-approved open-source license.

The license covers the original material in this repository only. Third-party dependencies keep their own licenses (see [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md), generated by `scripts/gen-notices.sh`). Claude Code is a separate product with its own terms; clawdyX only drives the copy you installed and logged into yourself.
