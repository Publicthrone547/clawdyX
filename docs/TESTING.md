# Testing

```sh
make test               # go test -race ./...  (no network, no real claude)
make lint               # gofmt, go vet (+ staticcheck/govulncheck if installed)
make check-secrets      # fails if a token-shaped string is tracked
make test-desktop       # desktop module (Go) + frontend type check and Vitest
make vet-windows        # type-checks both modules, tests included, for windows/amd64
make check-all          # all of the above
```

Windows code is compiled and vetted on every `make check-all`, but its tests cannot run on the Mac: the Windows-only paths (Run-key service, taskkill, Win32 process and disk queries, the socket ACL model) are untested until someone runs them on Windows.

## What the suites cover

| Package | Coverage |
| --- | --- |
| `internal/config` | Allowlist: exactly one owner, positive unique IDs, roles, labels, size cap, owner moved first, conflicting `owner_user_id` refused; `Rewrite` validates before replacing. `owner_user_id` must be a positive integer (0, negative, lists, `*`, usernames, other IDs, unknown fields rejected; 0600 checks; token format without echoing; symlinked workspace canonicalised; minimum deny list |
| `internal/observability` | Redaction of bot tokens (including inside `/bot<token>/` URLs), Anthropic keys, bearer headers, key=value secrets, emails; logger output redacted |
| `internal/sessions` | `/newf` in one transaction (busy refusal leaves the active session unchanged), name uniqueness, `/clear` copies tier/model/effort, forget/restore window, restore refused after history deletion; history lookup by the documented slug or by UUID in exactly one folder, ambiguous and symlinked transcripts refused, only the session's own two paths deleted |
| `internal/store` | v1→v2 migration keeps data and writes a pre-migration backup; tombstones; capture audit. Schema, duplicate update → `ErrDuplicate`, whitelisted session fields, stats, events bound, capability cache invalidated by CLI version, backup, permissions |
| `internal/policy` | observe/assisted flag mapping, secret-path deny rules, bypass and automation unreachable, broad allow rules rejected, path traversal, symlink escape, workspace swap |
| `internal/agent` | No prompt in argv, variadic flags last, env allowlist (no token, API key, nested markers), stream-json parsing, per-message usage dedupe, denials, unparseable lines, SIGINT records a result, ladder escalates to SIGKILL and kills grandchildren, PID-reuse guard, missing transcript, CLI helpers, model probe |
| `internal/jobs` | Per-session serialisation, positions, cancel queued/waiting/running, queue limit, global concurrency cap, timeout, effort warning resets to auto, missing session, failure mapping, pause, shutdown, recovery kills owned orphans and never re-runs |
| `internal/telegram` | Roles: a second user's answers go to their chat; 17 owner-only commands refused; a user cannot cancel the owner's job (command, `/stop`, button) or confirm the owner's token; strangers stay silent. Allowlist edits rewrite config.json (0600) and apply live; detection records nothing unless armed; token test/replace never echoes the token and restarts the worker. Lifecycle: `/newf`, `/clear`, `/rename`, `/sessions restore`, `/forget` only after the exact confirmation, `/forget --delete` removes only that transcript, `/newf --delete` refuses without one; socket methods for the same. Screen commands: disabled in config, `/screens` never captures, `/screen 2` and `/screen all` photos only to the owner, nothing captured at startup, `/screen off` blocks `/look` and is re-enabled only locally; `/look` runs Claude with `--input-format stream-json`; app captures and `screens.use` stay off Telegram. Control methods: status, bot stop/start (stopped bot ignores updates, 20 concurrent starts → one poller, events in order), sessions create/archive/restore/switch, effort/mode/model validation, jobs, processes, log tail redaction. Silent rejection of other users, group chats, foreign chat IDs, forged callbacks and edited messages; prompt → ack → final answer; duplicate updates; `/model` validation and `fable` as primary model; `/effort`; aliases; cancel button with forged nonce; `/stop all` confirmation with stale/replayed tokens; many commands; draft→edit fallback |
| `internal/control` | Socket 0600 in a 0700 dir, peer-uid rejection, typed errors (invalid params, unknown method, timeout, panic), live-socket takeover refused, oversize and garbage frames, ordered events to several clients, backpressure; `FuzzFrames` |
| `internal/screen` | Display ordering (main first) and durable keys, never-upscale JPEG encoding, broker switch/limits/TTL/discard/audit, access state from the last real capture, MCP screenshot on a second display maps clicks to global points, worker authorization before each screenshot |
| `internal/vault` | Memory store semantics, per-config Keychain accounts. Tests never touch the real Keychain (`TestMain` swaps in a memory store). |
| `cmd/clawdyx` | Windows Run-key command quoting and parsing; setup stores the token in the vault by default, `-token-file` writes a 0600 file, `-migrate-token` moves it and deletes the file only after a read-back, env > Keychain > file order |
| `desktop` (Go) | LaunchAgent plist parsing, home-path shortening, only allowlisted links can be opened, diagnostics report redaction, calls fail cleanly when the worker is down |
| `desktop/frontend` (Vitest) | Secret input masked until shown, status badges always carry a text label, allowlist ID validation, formatting helpers |
| `internal/setup` | Owner detection keeps private-chat humans only and acknowledges pending updates; workspace and name checks |
| `internal/render` | Escaping of model output, Markdown conversion, safe links only, chunk limits with balanced code fences |

Unit and end-to-end tests use `internal/agent/testdata/fakeclaude`, a small program that imitates `claude -p` stream-json (2.1.292 shapes), and an `httptest` fake of the Bot API.

## Live tests (real claude CLI, small cost)

```sh
CLAWDYX_LIVE_DIR=$(mktemp -d) go test -tags live -run TestLive -v ./internal/jobs/
```

These run eight short prompts against the installed CLI with the bot's real flags and verify:

- **Observe tier:** reads inside the workspace work, and writes are denied.
- **Outside the workspace:** reads are blocked for both the Read tool and Bash `cat`.
- **Assisted tier:** file creation works, and `rm` is denied.
- **Resume:** context survives a model switch to sonnet.

`TestLiveAutomation` checks the automation tier. It verifies three things: `open -a Firefox https://www.youtube.com` is allowed, a file outside the workspace can be read when asked, and `rm` is still denied.

`TestLiveScreenTools` (with `CLAWDYX_BIN=$PWD/bin/clawdyx`) checks that the real CLI starts the bot's screen MCP server in an automation session and that the screenshot tool is called rather than denied. On this Mac the screenshot succeeded, so one image of the main display was sent to Claude during the test.

Last run: 2026-10-16, CLI 2.1.292, all passed. An image prompt through `--input-format stream-json` (a generated red square) was verified on 2026-10-17: the CLI answered "Red" and exited at end of input.

## Desktop app checks (manual, on the target Mac)

1. Fresh account (or `mv ~/.config/clawdyx ~/.config/clawdyx.bak` and remove the Keychain item): the wizard appears; Continue stays disabled until the token test passes, a valid ID is entered, Claude is logged in and the folder is valid.
2. Start/Stop on Overview: the badge changes only after the worker confirms; twenty fast clicks never create a second poller (`clawdyx ctl status.get` → `bot.pollStarts`).
3. Close the window: the process keeps running, the menu-bar item stays, Telegram still answers. Quit → the dialog offers to stop the bot.
4. Monitors: nothing is captured until a Capture button is clicked; switching screen features off also blocks `/screen` in Telegram.
5. Keyboard: Tab reaches every control with a visible focus ring; Esc closes dialogs; ⌘1–7 switch screens. VoiceOver reads badges as text.

## Manual acceptance (needs a real bot token)

1. From the owner account, `/start`, `/status`, a plain prompt, `/model fable`, `/effort high`, `/stop` during a long prompt.
2. From a second Telegram account, send messages and press old buttons. Expect no reply and no new rows in `/jobs`.
3. `/screens`, `/screen 2`, `/look 2 what is on this screen?`, `/screen off`, then `/screen 2` again (expect the switched-off message).
4. Add the bot to a group and send `/status` there as the owner. Expect no reply.
5. `kill -TERM` the bot during a long job and restart it. Expect the "interrupted, not re-run" notice and no duplicate job.
6. Start a second `clawdyx run`. Expect exit code 3 with "another instance".
