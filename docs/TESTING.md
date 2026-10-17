# Testing

```sh
make test               # go test -race ./...  (no network, no real claude)
make lint               # gofmt, go vet (+ staticcheck/govulncheck if installed)
make check-secrets      # fails if a token-shaped string is tracked
```

## What the suites cover

| Package | Coverage |
| --- | --- |
| `internal/config` | `owner_user_id` must be a positive integer (0, negative, lists, `*`, usernames, other IDs, unknown fields rejected; 0600 checks; token format without echoing; symlinked workspace canonicalised; minimum deny list |
| `internal/observability` | Redaction of bot tokens (including inside `/bot<token>/` URLs), Anthropic keys, bearer headers, key=value secrets, emails; logger output redacted |
| `internal/store` | Schema, duplicate update → `ErrDuplicate`, whitelisted session fields, stats, events bound, capability cache invalidated by CLI version, backup, permissions |
| `internal/policy` | observe/assisted flag mapping, secret-path deny rules, bypass and automation unreachable, broad allow rules rejected, path traversal, symlink escape, workspace swap |
| `internal/agent` | No prompt in argv, variadic flags last, env allowlist (no token, API key, nested markers), stream-json parsing, per-message usage dedupe, denials, unparseable lines, SIGINT records a result, ladder escalates to SIGKILL and kills grandchildren, PID-reuse guard, missing transcript, CLI helpers, model probe |
| `internal/jobs` | Per-session serialisation, positions, cancel queued/waiting/running, queue limit, global concurrency cap, timeout, effort warning resets to auto, missing session, failure mapping, pause, shutdown, recovery kills owned orphans and never re-runs |
| `internal/telegram` | Silent rejection of other users, group chats, foreign chat IDs, forged callbacks and edited messages; prompt → ack → final answer; duplicate updates; `/model` validation and `fable` as primary model; `/effort`; aliases; cancel button with forged nonce; `/stop all` confirmation with stale/replayed tokens; many commands; draft→edit fallback |
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

Last run: 2026-10-16, CLI 2.1.292, all passed.

## Manual acceptance (needs a real bot token)

1. From the owner account, `/start`, `/status`, a plain prompt, `/model fable`, `/effort high`, `/stop` during a long prompt.
2. From a second Telegram account, send messages and press old buttons. Expect no reply and no new rows in `/jobs`.
3. Add the bot to a group and send `/status` there as the owner. Expect no reply.
4. `kill -TERM` the bot during a long job and restart it. Expect the "interrupted, not re-run" notice and no duplicate job.
5. Start a second `clawdyx run`. Expect exit code 3 with "another instance".
