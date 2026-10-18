# clawdyX architecture and verified Claude Code behaviour

This document records how clawdyX drives the Claude Code CLI and why. Sections 1–3 hold the findings and the capability matrix that were **verified** by running Claude Code 2.1.292 on macOS 27 (Apple silicon) in October 2026. Sections 4–12 are the design, 13–19 the build, test and operations plan. It started as the pre-implementation specification; the "Implementation status" section at the end lists where the code differs.

| Item | Value at verification time |
| --- | --- |
| Target machine | macOS 27.0.1, Apple silicon, zsh |
| Claude Code | 2.1.292 (Homebrew cask, native arm64), logged in with a claude.ai subscription |
| Language | Go 1.27 |
| Authorized Telegram sender | the numeric user ID configured locally, private chat only |

How to read this document: Sections 1–3 are the findings and the capability matrix (what is *verified* on this machine). Sections 4–12 are the design. Sections 13–16 are the build plan, tests, runbook, and roadmap. Appendices hold the raw (redacted) evidence and the JSON shapes the implementer must parse. Every claim that was **not** verified locally is labelled `UNVERIFIED` or `DOCS-ONLY`.

---

## Table of contents

1. Executive recommendation, scope, non-goals
2. Environment audit findings
3. Verified Claude CLI capability matrix
4. Integration strategy (architecture decision record)
5. Architecture and data flow
6. Session, job, and state model
7. Command reference (syntax, semantics, examples, errors)
8. Model and effort controls
9. Chat UX, streaming, and long-running jobs
10. Computer-control scope and the capability registry
11. Process and session monitoring
12. Go dependencies and decisions
13. Configuration contract, state paths, logging, LaunchAgent
14. Security: threat model, authorization gate, confirmations, redaction, data classification, risk register
15. Performance targets and benchmarks
16. Testing strategy and acceptance criteria
17. Implementation phases, files, and local Git workflow
18. Operational runbook
19. Prioritized roadmap
20. Open questions
- Appendix A: Evidence log (redacted)
- Appendix B: Observed JSON shapes (CLI 2.1.292)
- Appendix C: References checked (with dates)
- Appendix D: Glossary of identifiers

---

## 1. Executive recommendation, scope, non-goals

### 1.1 Recommendation

Build a single Go binary, `clawdyx`, that long-polls Telegram and drives the already-installed, already-authenticated `claude` CLI as **one short-lived child process per user message**:

```
claude -p --output-format stream-json --verbose --include-partial-messages \
       --permission-mode <mode> --permission-prompts none \
       --model <m> [--effort <e>] (--session-id <uuid> | --resume <uuid>) \
       --setting-sources user --strict-mcp-config [--allowedTools …] [--disallowedTools …]
```

with the prompt delivered on **stdin** (never argv), the working directory set to the selected workspace, and output parsed from the stream-json events. This path was verified end-to-end on the target machine (Section 3 and Appendix A): it reuses the local claude.ai login (also under a launchd-like empty environment), preserves conversation context across processes via `--resume`, allows per-message model/effort changes, resumes from any directory, can be cancelled with SIGINT/SIGTERM while keeping the transcript resumable, and exposes session IDs, usage, cost estimates, permission denials, and (undocumented) rate-limit windows.

Why not the alternatives (details in Section 4): the Agent SDK requires API-key auth and Anthropic's docs explicitly disallow third-party apps from offering claude.ai login, so it cannot meet the "reuse the local subscription" requirement; a persistent interactive process saves ~1.5–3 s per turn but relies on an undocumented control protocol, holds ~300 MB per idle session, and complicates cancellation, model switching, and crash recovery; `claude --bg` background sessions cannot be combined with `-p` and are a TUI-oriented research preview; transcript scraping is version-fragile and is used only as a read-only *existence* check.

### 1.2 Scope of the MVP (Phase 1)

- Single authorized operator (`<owner id>`), private chat only, silent rejection of everything else.
- Natural-language prompts routed into a selected logical session; `/new`, `/sessions`, `/switch`.
- `/start`, `/help`, `/status`, `/usage`, `/stop`, `/cancel`, `/jobs`, `/processes`, `/model`, `/effort`, `/doctor`.
- Streaming progress (Telegram `sendMessageDraft` when available, edited progress message otherwise), chunked final answers, inline Cancel button.
- Two permission tiers per session: `observe` (read-only) and `assisted` (edits inside the workspace plus an explicit command allowlist). No bypass mode anywhere.
- SQLite state, single-instance lock, idempotent restart recovery, LaunchAgent template, redacted structured logs.

### 1.3 Non-goals

- Public or multi-user bot, group chats, inline mode, webhooks, any listening port.
- Running while the user is logged out (the login keychain is required for claude.ai auth; LaunchAgents only run in the logged-in GUI session).
- Arbitrary shell execution from Telegram, `--dangerously-skip-permissions`, `bypassPermissions`, sudo, system-settings changes.
- Desktop GUI automation in the MVP (evaluated in Section 10.5 as an optional later phase with explicit macOS permission requirements).
- Changing the owner's own Claude Code settings (`~/.claude/settings.json`), logging in/out, or managing credentials.
- Any Git remote, GitHub connection, or push. Local commits only (Section 17.4).

---

## 2. Environment audit findings

All checks were read-only except the bounded `claude -p` capability tests listed in Appendix A.1 (trivial prompts, scratch working directories). Sensitive values are redacted.

### 2.1 Machine and toolchain

| Item | Finding | Evidence |
| --- | --- | --- |
| macOS | 27.0.1, build 26A434, Darwin 27.0.0, arm64 | `sw_vers`, `uname -srm` |
| CPU / RAM | Apple M3 Max, 14 cores, 36 GiB | `sysctl` |
| User | `<you>`, uid 501, `$SHELL=/bin/zsh`, `$HOME=/Users/<you>` | `id`, env |
| Go | go1.27.0 darwin/arm64 at `/opt/homebrew/bin/go` (Homebrew `go 1.27.0`); `GOPATH=/Users/<you>/go`, `GOTOOLCHAIN=auto`, `CGO_ENABLED=1`, Xcode at `/Applications/Xcode.app`; `gopls` installed; `golangci-lint`/`staticcheck` absent | `go env`, `brew list` |
| Node | v26.7.0 (not needed; the CLI is a native binary) | `node --version` |
| Coreutils gaps | `timeout` is **not** installed (BSD userland); `perl`, `jq`, `sqlite3`, `curl`, `lsof`, `pgrep`, `ps`, `top`, `vm_stat`, `sysctl`, `osascript`, `shortcuts`, `caffeinate`, `launchctl`, `log` present | `command -v` |
| launchd user env | `launchctl getenv PATH` is empty and the gui domain exports only `SSH_AUTH_SOCK`; LaunchAgents therefore get the default `PATH=/usr/bin:/bin:/usr/sbin:/sbin` (no `/opt/homebrew/bin`) | `launchctl print gui/501` |
| Existing LaunchAgents | Google Updater/Keystone, `homebrew.mxcl.postgresql@18`; nothing Claude- or bot-related | `ls ~/Library/LaunchAgents` |
| Other | Telegram Desktop is running (`ru.keepcoder.Telegram`); irrelevant to the design | `launchctl list` |

### 2.2 Repository

`/Users/<you>/projects/clawdyX` contained only the owner's private analyst brief. It is **not** a Git repository, has no `.claude/` directory, no Go module, no prior Telegram code, and therefore no conventions to inherit. Nothing is reusable; the implementation starts from scratch (Section 17).

### 2.3 Claude Code installation and authentication

| Item | Finding |
| --- | --- |
| Path | `/opt/homebrew/bin/claude` → `/opt/homebrew/Caskroom/claude-code/2.1.292/claude` (symlink; cask installed 2026-10-16) |
| Binary | Mach-O 64-bit arm64 executable, 224 MB, no Node dependency |
| Version | `2.1.292 (Claude Code)`; `claude doctor`: commit `37832d0b7cad`, platform darwin-arm64, package manager homebrew, "Auto-updates: Managed by package manager", "No installation issues found" |
| Auth | `claude auth status` → `loggedIn: true`, `authMethod: "claude.ai"`, `apiProvider: "firstParty"`, `subscriptionType: "max"`, `configDirectory: /Users/<you>/.claude`, `projectsDirectory: /Users/<you>/.claude/projects`. Output also contains `email`, `orgId`, `orgName` (**must be redacted** by the bot). No API key configured (`~/.claude.json` has `oauthAccount`, no `primaryApiKey`); stream-json `system/init` reports `apiKeySource: "none"` |
| Credential storage | macOS Keychain per docs; **not read** during analysis. A child `claude -p` started with `env -i HOME=… PATH=/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/Caskroom/claude-code/2.1.292` authenticated successfully, so a LaunchAgent in the logged-in GUI session can reuse the login |
| User settings (`~/.claude/settings.json`) | `model: "fable"`; `modelSettings.effortLevel`: `claude-fable-5-1: high`, `claude-opus-5: high`, `claude-sonnet-5: medium`, `claude-opus-5-5: high`; ten hook events (`SessionStart`, `PreToolUse`, … ) all invoking `/Users/<you>/.config/iterm2/cc-status`; plugins `dart-flutter`, `frontend-design`, `gopls-lsp`, `cc-plugin-you-should-know`; `skipDangerousModePermissionPrompt: true`; no `permissions` block (sessions start in `default`/Manual mode) |
| User-scope MCP servers | two: `plugin:dart-flutter:dart-mcp-server` (status `failed` in a `-p` run) and `claude.ai Claude Docs` (`connected`) |
| Transcripts | `~/.claude/projects/<cwd with every non-alphanumeric char replaced by '-'>/<session-uuid>.jsonl`, mode 0600, ~150–185 KB each even for one-turn sessions; a `memory/` subfolder per project |
| Live-session registry | `~/.claude/sessions/<pid>.json` + `.key` files containing a `peerToken` — **never read or copy these**; use `claude agents --json` instead |
| Nested execution | The analysis ran inside a Claude Code session (`CLAUDECODE=1`, `CLAUDE_CODE_*` set). `claude -p` still worked and persisted/resumed sessions. The bot must nevertheless build a clean child environment (Section 13.4) |
| Running Claude processes | two interactive sessions (RSS ≈ 465 MB and 645 MB); a `-p` text-only job peaked at ≈ 343 MB RSS |

### 2.4 Expected run mode

The bot runs as a **LaunchAgent in the logged-in GUI session** (`gui/501`), or manually from a terminal during development. A LaunchDaemon (system domain, runs while logged out) is **not viable**: it would not have the user's keychain, so the claude.ai login would be unavailable.

---

## 3. Verified Claude CLI capability matrix (2.1.292)

Legend: **V** = verified by running it on this machine on 2026-10-16; **H** = present in `claude --help` output; **D** = stated by current official docs (fetched 2026-10-16); `UNVERIFIED` = not exercised.

| Capability | Status | Observed behaviour / contract | Bot usage and fallback |
| --- | --- | --- | --- |
| `claude -p` (print mode) | V | Exit 0 on success; invalid flags → stderr + non-zero before the run; in-run failures are reported *inside* the JSON result (`is_error: true`) with exit 0 (model not found) or 1 (budget) | Primary execution path |
| Prompt on stdin | V | `echo "…" \| claude -p …` works; docs cap piped stdin at 10 MB | Always use stdin: avoids argv length limits, prevents prompt text from appearing in `ps`, and avoids the variadic-flag trap below |
| Variadic flags (`--tools`, `--allowedTools`, `--disallowedTools`, `--add-dir`, `--mcp-config`, `--betas`) | V | `claude -p --tools "" "prompt"` fails with `Input must be provided either through stdin or as a prompt argument` because the positional prompt is swallowed; `--` separator or stdin fixes it | Never pass a positional prompt; stdin only |
| `--output-format json` | V | Single result object (fields in Appendix B.1): `session_id`, `result`, `is_error`, `subtype`, `terminal_reason`, `num_turns`, `duration_ms`, `duration_api_ms`, `usage`, `modelUsage` (**cumulative over the whole session when resuming**), `total_cost_usd` (client-side list-price estimate), `permission_denials`, `api_error_status`, `stop_reason`, `uuid` | Used for probes; jobs use stream-json |
| `--output-format stream-json --verbose` | V | NDJSON events: `system/hook_started`, `system/hook_response` (SessionStart hooks always shown), `system/init` (Appendix B.2), `system/status` (`status: "requesting"`), `system/permission_denied`, `assistant` (with per-message `message.usage`), `user` (tool results), `rate_limit_event`, `result` | Primary output mode |
| `--include-partial-messages` | V | Adds `stream_event` lines: `message_start`, `content_block_start`, `content_block_delta` (`text_delta`), `content_block_stop`, `message_delta`, `message_stop` | Token-level streaming to Telegram |
| `--input-format stream-json` | V | Multiple `{"type":"user","message":{"role":"user","content":"…"}}` turns in one process work (same `session_id`, one `result` per turn); `{"type":"control_request","request_id":"…","request":{"subtype":"interrupt"}}` is acknowledged with `control_response` and ends the turn with `terminal_reason: "aborted_streaming"`. `--permission-prompts host` did **not** emit a `control_request` for a prompting tool (it emitted `system/permission_denied`); the SDK's initialize/hook handshake is undocumented for CLI users | Optional later optimisation (Section 4.4); not MVP |
| `--session-id <uuid>` | V | Session created with the given UUID; reusing a UUID → exit 1, `Error: Session ID <uuid> is already in use.` | Bot pre-generates IDs for new sessions (idempotency, recovery) |
| `--resume <uuid>` | V | Context preserved (secret-word test); works from a **different cwd**; `--model` on resume is accepted and the new model is used; unknown ID → exit 1, stderr `No conversation found with session ID: …`, no JSON | Every follow-up message |
| Concurrent resume | V | Two processes resuming the same session simultaneously both succeed; the transcript ends up interleaved. **The CLI does not lock sessions** | Bot enforces a per-session mutex and FIFO queue |
| `--fork-session` | V | New UUID with the old context; original remains resumable | `/fork` (roadmap) |
| `--continue` | H/D | Most recent conversation in cwd; `-p --continue` includes `-p` sessions | Not used (ambiguous with several sessions per workspace) |
| `--model <alias\|id>` | V | `fable`→`claude-fable-5-1` (contextWindow 1,000,000, maxOutputTokens 64,000), `opus`→`claude-opus-5-5`, `sonnet`→`claude-sonnet-5-5`, `haiku`→`claude-haiku-4-5-20251001`, `fable[1m]`→`claude-fable-5-1`. Invalid name → run proceeds, result `is_error: true`, `api_error_status: 404`, `terminal_reason: "api_error"`, stderr `[claude-code:unrecognized_model] {…}`, exit 0, ~4 s, `total_cost_usd: 0` | Validation probe + error mapping (Section 8) |
| `--effort <level>` | V | Help lists `low, medium, high, xhigh, max`. An invalid value (including `auto` and, per help, `ultracode`) prints `Warning: Unknown --effort value '…' — ignoring it and using the default effort.` **and still runs**. No output field reports the effective effort (`system/init` only has `per_turn_effort_active: true`) | Bot validates before launch, scans stderr for the warning, tracks effort itself |
| `--advisor <model>` | V(accepted) | Not listed in `--help` but accepted (`--advisor fable` ran, exit 0). Docs: a separate server-side advisor tool; `fable` here means "Fable as advisor", distinct from `--model` | **Not used**. `/model fable` sets the primary model only |
| `--permission-mode` | V | Choices `acceptEdits, auto, bypassPermissions, manual, dontAsk, plan` (`manual` = docs' `default`); invalid → exit error before the run. Without the flag the mode can be `auto` on some accounts (docs) — observed `permissionMode: "default"` here | Always passed explicitly |
| `--permission-prompts none` | V | With `manual`: Bash `touch` and `Write` were denied (files not created), `permission_denials[]` populated with `tool_name`, `tool_use_id`, `tool_input`; read-only commands (`ls`, `git status --short`) ran without prompts | Always passed (MVP has no permission host) |
| `--permission-mode dontAsk` + `--disallowedTools Edit,Write,NotebookEdit` | V | `git status --short` ran; `touch` denied; Write tool absent ("DENIED") | **observe** tier |
| `--permission-mode acceptEdits` | V | Write auto-approved inside cwd; docs: also auto-approves `mkdir, touch, rm, rmdir, mv, cp, sed` inside the working directories | **assisted** tier, with scoped `Bash(rm *)` deny rules |
| `--permission-mode plan` in `-p` | V | Wrote a plan file under `~/.claude/plans/` and replied that `ExitPlanMode` is unavailable | **Not used** (side effects, awkward UX) |
| `--restricted` | V | Removes Bash/code tools and WebFetch, confines file tools to working dirs, ignores user/project/local settings, refuses bypass; "CANNOT" when asked to run `ls` | Optional hardening flag for observe (`workspace.strict_observe`) |
| `--allowedTools "Bash(ls:*)"` | V | Command ran (inconclusive because `ls` is read-only anyway); docs specify `Bash(git diff *)` (space-star) as canonical, colon form equivalent | Use space-star form; test with a non-read-only command |
| `--disallowedTools Bash` | V | Tool removed from context ("CANNOT") | observe tier |
| `--max-budget-usd` | V | `0.001` → exit 1, `subtype: "error_max_budget_usd"`, `terminal_reason: "budget_exhausted"`, `result: null`; the first turn of a fresh session costs ≈ $0.22–0.31 *list-equivalent* because of prompt-cache creation | Optional safety cap ≥ $2; not billed on Max but still enforced |
| `--max-turns` | H/D | Exits with an error when reached | Probes use `--max-turns 1`; jobs use a configurable cap |
| `--no-session-persistence` | V | Accepted with `-p` | Model probes only |
| `--name` | V(accepted) | Accepted with `-p`; the name did not appear in `claude agents --json --all` (expected: not a background session). Name-based resume `UNVERIFIED` | Optional cosmetic |
| `--setting-sources`, `--strict-mcp-config` | V | Accepted; `--setting-sources project --strict-mcp-config` still loaded the user `model` default in one run (`UNVERIFIED` whether `project` alone excludes user settings; the bot passes `user` explicitly) | Section 13.4 |
| `--bare` | H/D | Fast start, but "Anthropic auth is strictly ANTHROPIC_API_KEY or apiKeyHelper … OAuth and keychain are never read". Docs: "will become the default for `-p` in a future release" | **Never pass.** Version-change smoke probe guards against a future default (Section 18.6) |
| `--bg`, `claude agents`, `attach`, `logs`, `stop\|kill`, `rm`, `respawn`, `daemon` | H/D (+V for `agents --json`, `daemon status`) | `--bg` cannot combine with `-p` (docs); `claude agents` needs a TTY; `claude agents --json` works headless and lists **interactive and background** sessions (`pid, cwd, kind, startedAt, sessionId, name, status`); `--all` adds completed background sessions; `claude daemon status` → `not running` (hidden command). Research preview per docs | `/processes` data source (cached); `claude stop <id>` only for background sessions after confirmation (Phase 2) |
| `claude auth status [--json\|--text]` | V | JSON by default; exit 0 logged in / 1 not; **slow: 19–36 s** per call | Background refresh every 15 min + at startup; never on the request path |
| `claude agents --json` latency | V | **10–15 s** per call, even idle | Background refresh every 60 s; `/processes` shows cache age |
| `claude doctor` | V | Read-only, prints and exits, ~0.8 s, no JSON mode | `/doctor` includes its text (redacted) |
| `claude --version` | V | 0.01 s | Startup + hourly version check |
| `claude purge`, `claude update\|upgrade\|install`, `claude auth login\|logout`, `setup-token`, `import`, `mcp`, `plugin` | H | State-changing | **Never invoked by the bot** |
| Signals | V | SIGINT: turn ends, result recorded (`subtype: "error_during_execution"`, `terminal_reason: "aborted_streaming"`), exit 0. SIGTERM: exit 143, no result, docs: Bash process tree killed, `SessionEnd` hooks run. SIGKILL: exit 137. In all three cases the transcript was persisted and `--resume` worked afterwards. No child processes were present for text-only jobs | Cancel = SIGINT → SIGTERM → SIGKILL to the process group |
| Startup overhead | V | Wall minus `duration_ms` ≈ 1.3–2.9 s per `-p` process (hooks, plugins, MCP, skills discovery); `time_to_request_ms` 33–371 ms | Performance budget (Section 15) |
| `rate_limit_event` | V (undocumented) | `rate_limit_info.unifiedWindows.{five_hour,seven_day,…}.{utilization,resetsAt}`, `status`, `rateLimitType`, `overageStatus` | `/usage` shows it labelled "observed, undocumented" |
| Hooks in `-p` | V | The owner's `SessionStart` hook ran in every `-p` process (`hook_response.exit_code: 0`) | Keep user settings (consistency) unless `claude.setting_sources` is changed |
| Background work at exit | D | A `-p` run may stay open up to 10 min waiting for background tasks; `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS` caps it | Set to 60000 ms in the child env; job timeout is the hard cap |
| Agent SDK (TS/Python) | D | "Anthropic does not allow third party developers to offer claude.ai login … Use the API key authentication methods" | Rejected for this project |

---

## 4. Integration strategy (architecture decision record)

### 4.1 Options compared

| Option | Reuses local claude.ai login | Context continuity | Model/effort per message | Cancellation | Permission control | Complexity / fragility | Verdict |
| --- | --- | --- | --- | --- | --- | --- | --- |
| **A. One `claude -p` process per message, `--resume`** | Yes (verified, also under `env -i`) | Yes via session ID (verified across processes and directories) | Yes (flags per invocation; verified `--model` on resume) | Kill the process group; transcript stays resumable (verified for INT/TERM/KILL) | `--permission-mode`, allow/deny rules, `--permission-prompts none` (verified) | Low. Documented headless surface. Cost: 1.3–2.9 s startup per message, hooks/plugins load each time | **Chosen (MVP)** |
| B. Persistent `-p --input-format stream-json` process per session | Yes | Yes (verified two turns in one process) | No: flags are per process; would need undocumented `control_request` subtypes | `control_request interrupt` (verified) or kill | Same flags; host-side prompt relay needs the SDK's undocumented handshake (observed: denied instead of relayed) | Medium-high: stdin/stdout framing, idle eviction (~275–345 MB RSS each), respawn on crash, orphan cleanup on bot restart | Optional Phase 3 optimisation behind a flag |
| C. Claude Agent SDK (TypeScript/Python) | **No** (API key required; claude.ai login disallowed for third-party apps per docs) | Yes | Yes | Yes | `canUseTool` callback (best UX) | Adds a second runtime (Node/Python) | Rejected: violates the auth requirement |
| D. `claude --bg` background sessions + `claude logs/stop` | Yes | Yes | Yes at start | `claude stop` | Same | Cannot combine with `-p`; TUI/attach oriented; research preview; `claude agents --json` takes 10–15 s | Visibility only (`/processes`), management in Phase 2 |
| E. Parse `~/.claude/projects/*.jsonl` transcripts | n/a | n/a | n/a | n/a | n/a | Undocumented format, 150 KB+ files, privacy exposure | Only a read-only *file existence* check for `/resume <external-id>` validation |
| F. Interactive PTY (`claude` TUI) | Yes | Yes | Via `/model` text commands | Fragile | Prompts need a human | ANSI scraping; brittle | Rejected |

### 4.2 Decision

Option A. Measured per-message overhead is acceptable for a chat UI (typical first-token latency 2–5 s including the API round trip; an immediate Telegram acknowledgement hides it), and every other property the brief requires (predictable cancellation, per-message model/effort, restart recovery without duplicate prompts, auditability) is simplest with a stateless process per message.

Consequences the implementer must handle:

1. **Serialize per session.** The CLI does not lock a session (verified). A per-session mutex plus a bounded FIFO queue is mandatory.
2. **Usage accounting is per-session-cumulative in the `result` event** when resuming. Compute per-job tokens by summing `assistant` events' `message.usage` from the stream; use `result.usage`/`total_cost_usd` only as the session total.
3. **Effort is not echoed back.** The bot is the source of truth for the effective effort and must validate before launching (the CLI silently ignores bad values).
4. **Startup cost.** Do not spawn processes for anything but real prompts and explicit probes. `/status`, `/jobs`, `/sessions`, `/model`, `/effort` must answer from local state in milliseconds.

### 4.3 Permission model decision

- `bypassPermissions` / `--dangerously-skip-permissions`: never (also enforce `permissions.disableBypassPermissionsMode` through `--settings` JSON passed by the bot, so even a misconfiguration cannot enable it).
- `observe` (default for new sessions): `--permission-mode dontAsk --permission-prompts none --disallowedTools Edit Write NotebookEdit WebFetch WebSearch` plus secret-path deny rules (Section 14.6). Reads inside the workspace and the built-in read-only Bash set work; everything else is denied and reported.
- `assisted`: `--permission-mode acceptEdits --permission-prompts none --allowedTools <workspace allowlist> --disallowedTools <workspace denylist, default: Bash(rm *) Bash(rmdir *) Bash(sudo *) Bash(git push *) Bash(git remote *) Bash(curl *) Bash(wget *) …>`. Edits and `mkdir/touch/mv/cp/sed` inside the workspace are auto-approved (documented), allowlisted commands run, everything else is denied.
- `automation` (Phase 3, off by default): `--permission-mode auto --permission-prompts none` with the same deny list. Claude Code's classifier (`claude auto-mode config`) reviews commands; hard-deny categories remain. Enabled only when `modes.automation_enabled: true` and the owner selects it per session with a confirmation.
- Interactive approvals (Phase 3): `--permission-prompt-tool mcp__claude_tg__approve` served by a tiny MCP stdio helper (`clawdyx mcp-approve`) that relays each request to Telegram with Allow/Deny buttons and a timeout (deny on timeout). Contract in Section 10.4.

### 4.4 Future option B (warm sessions)

If per-message latency becomes the main complaint, add `claude.warm_sessions: true`: keep one `--input-format stream-json` process per *active* session, evict after `warm_idle_timeout` (default 10 min) or when memory pressure is high, and fall back to Option A whenever model/effort/mode changes (those require a new process). Interrupt via `control_request` (capability `interrupt_receipt_v1` is advertised in `system/init.capabilities`, so feature-detect it). Treat the control protocol as undocumented: guard every message type with tolerant parsing and a kill-switch.

---

## 5. Architecture and data flow

### 5.1 Components

```
cmd/clawdyx/main.go              executable: flags (run | check | service install|uninstall|status | backup | mcp-approve)
internal/config                    load + validate config.json and env; fail closed
internal/telegram                  long polling, update gate (authz), command parsing, outbound sender (rate limit, retries), formatting/chunking
internal/agent                     Claude CLI adapter: argv builder, child env, process group, stream-json parser, event model, probes
internal/sessions                  logical sessions, workspace binding, model/effort/mode preferences, external-session validation
internal/jobs                      job registry, per-session queues, global semaphore, state machine, cancellation, timeouts, recovery
internal/processes                 bot-owned process table, `ps` sampling, `claude agents --json` cache, system metrics
internal/policy                    workspace roots (canonical paths, symlink checks), tiers → CLI flags, capability registry, confirmations
internal/store                     SQLite (modernc), migrations, retention, backups
internal/observability             slog JSON logging, redaction writer, metrics counters, health summary
internal/render                    Telegram message rendering: HTML escaping, chunking, code fences, tables → monospace
internal/version                   CLI version detection, capability cache keyed by version
```

### 5.2 Data flow (one prompt)

```
Telegram ──getUpdates (long poll, 30 s)──▶ telegram.Poller
   │ update
   ▼
telegram.Gate  (from.id == <owner id> && chat.type == private && chat.id == from.id; else drop silently)
   │ authorized update
   ▼
telegram.Router ──/command──▶ command handlers (local state; ms)
   │ plain text
   ▼
jobs.Submit(sessionID, prompt) ──▶ store: INSERT job(queued, update_id, message_id)  (idempotent on update_id)
   │                                 reply: "⏳ j-17 queued/running · S3 · fable/high · [Cancel]"
   ▼
jobs.Worker (per-session mutex + global semaphore)
   │
   ▼
agent.Runner.Start(JobSpec)
   argv = claude -p --output-format stream-json --verbose --include-partial-messages
          --permission-mode dontAsk|acceptEdits --permission-prompts none
          --model <m> [--effort <e>] (--session-id U | --resume U)
          --setting-sources user --strict-mcp-config --settings '{"permissions":{"disableBypassPermissionsMode":"disable"}}'
          [--allowedTools …] [--disallowedTools …] [--max-turns N] [--max-budget-usd B]
   cwd = workspace path; env = minimal allowlist; stdin = prompt; Setpgid = true
   │ stream-json events
   ▼
agent.StreamParser ──▶ jobs: progress (text deltas, tool uses, denials, rate_limit_event, result)
   │                                   │
   │                                   ▼
   │                        telegram.Streamer: sendMessageDraft every ≥1 s (or editMessageText fallback),
   │                        sendChatAction typing every 4 s, final chunked sendMessage(s), footer
   ▼
store: UPDATE job(state, pid, usage, result_preview, terminal_reason) ; audit row
```

Cancellation: `/stop`, `/cancel j-17`, inline Cancel callback, or Telegram `stopped_message_generation` → `jobs.Cancel(id)` → SIGINT to the process group → after 10 s SIGTERM → after 5 s SIGKILL → job state `cancelled`; the Claude session remains resumable.

### 5.3 Concurrency model

- One poller goroutine; one dispatcher goroutine processes updates **in order** (library option `WithNotAsyncHandlers`), so command handling never races.
- One outbound sender goroutine with a token bucket (1 msg/s per chat, burst 3) and 429 `retry_after` handling.
- Jobs: global semaphore `max_concurrent_jobs` (default 2, max 4); per-session FIFO queue (`max_queue_per_session`, default 5); each running job has a reader goroutine for stdout (NDJSON) and one for stderr (bounded ring buffer, scanned for warnings).
- Background refreshers: `claude agents --json` every 60 s (skipped while ≥ `max_concurrent_jobs` jobs run), `claude auth status` every 15 min, `claude --version` hourly, `ps` sampling on demand (32 ms measured).
- Shutdown: on SIGTERM/SIGINT, stop polling, finish sending queued Telegram messages (≤ 5 s), cancel running jobs (SIGINT → TERM → KILL, ≤ 20 s total), mark them `interrupted`, close the DB, release the lock. launchd `ExitTimeOut` is set to 30 s.

### 5.4 Single instance

`flock` on `<state_dir>/bot.lock` (gofrs/flock). A second instance exits with code 3 and a clear stderr message. Telegram's `409 Conflict: terminated by other getUpdates request` is logged at ERROR with backoff and surfaced in `/status` as "duplicate poller detected at <time>" (it means a second instance or a leaked token).

---

## 6. Session, job, and state model

### 6.1 Identifiers (kept distinct everywhere)

| Identifier | Format | Owner | Example |
| --- | --- | --- | --- |
| Telegram user/chat ID | int64 | Telegram | `<owner id>` |
| Telegram `update_id`, `message_id` | int64 | Telegram | `message_id 4812` |
| Logical session ID | `S<n>` (bot counter) + optional name | bot | `S3`, name `telegram-bot` |
| Claude session ID | UUID v4 | bot-generated for new sessions, CLI-generated otherwise | `c6f4f93c-…` (shown truncated `c6f4f93c`) |
| Job ID | `j-<n>` | bot | `j-17` |
| OS PID / PGID | int | kernel | `15017` |
| Confirmation token | 6 chars base32 | bot | `K7Q2MD` |
| Background session short ID | string from `claude agents --json` | Claude Code | `7c5dcf5d` |

### 6.2 Entities

```
Settings      key/value: default_model, default_effort(auto), default_workspace, default_mode(observe),
              paused(false), schema_version, last_update_id, cli_version_seen
Session       id, name, claude_session_id (UUID), workspace_name, workspace_path (canonical), mode,
              model (NULL = chat default), effort (NULL = auto), status {active, archived, missing},
              origin {bot, attached}, created_at, last_used_at, turns, last_job_id
Job           id, session_id, kind {prompt, probe, capability}, state, tg_update_id (UNIQUE), tg_message_id,
              tg_ack_message_id, prompt_sha256, prompt_preview (≤80 chars, optional), model, effort, perm_mode,
              pid, pgid, proc_start_unix, started_at, finished_at, exit_code, terminal_reason, is_error,
              num_turns, in_tokens, out_tokens, cache_read_tokens, cache_create_tokens, cost_usd_est,
              result_preview (≤2 KB), error_summary (redacted), cancel_requested_at
JobEvent      job_id, seq, ts, type {tool_use, tool_result_summary, denial, status, stderr_warning, rate_limit},
              payload_redacted (≤1 KB), bounded to 500 rows per job
Confirmation  token, action, args_json, args_hmac, created_at, expires_at (+120 s), used_at, tg_message_id
Alias         name (lowercase, [a-z0-9_-]{1,24}), model, created_at
ProcessedUpdate update_id, received_at   (dedupe; pruned after 7 days)
Audit         ts, category, action, target, outcome, job_id   (never prompts or secrets)
CapabilityCache key (e.g. model:fable), value_json, cli_version, checked_at
```

### 6.3 Job state machine

```
queued ──start──▶ running ──result event──▶ completed | failed (is_error) | timeout
   │                 │
   │ /cancel         │ /cancel, Cancel button, stopped_message_generation
   ▼                 ▼
cancelled          cancelling ──process exit──▶ cancelled
                     │
running ──bot restart──▶ interrupted  (no automatic re-run; owner is notified)
```

| Transition | Side effects |
| --- | --- |
| submit → queued | Insert job (UNIQUE `tg_update_id` makes re-delivery idempotent); ack message with Cancel button |
| queued → running | Acquire session mutex + global slot; persist `pid`, `pgid`, `proc_start_unix` **before** reading output |
| running → completed/failed | Parse `result`; persist usage/terminal_reason; send final message(s); session `turns++`, `last_used_at` |
| running → timeout | Job timeout elapsed: SIGINT→TERM→KILL; message "⏱ j-17 timed out after 15m; session remains resumable" |
| → cancelled | Same signal ladder; message "⛔ j-17 cancelled (partial output kept in /logs j-17)" |
| → interrupted | On startup: for each `running` job, if `pid` is alive **and** its start time equals `proc_start_unix` and `claude agents --json` (or `ps` args) shows the same session/cwd, terminate the group; else just mark interrupted. Notify: "Bot restarted; j-17 was interrupted. Send the prompt again if needed (it was not re-run)." |

### 6.4 Session lifecycle

| Event | Behaviour |
| --- | --- |
| First plain message with no active session | Create `S1` in `default_workspace`, mode `observe`, pre-generate Claude UUID; launch with `--session-id` |
| `/new [name]` | Create session in the *current default workspace* (or `--ws <name>`); becomes active |
| `/switch S2` | Marks S2 active for the chat; no process is started |
| Resume failure (`No conversation found with session ID`) | Mark session `missing`; reply with the reason and suggest `/new`; never silently start a fresh session under the same label |
| `/resume <external uuid>` (Phase 2) | Validate ownership: the transcript must exist under `~/.claude/projects/<escaped dir>/<uuid>.jsonl` whose escaped directory starts with the escaped form of an allowed workspace root; or the UUID is listed by `claude agents --json` with a `cwd` inside an allowed root. Then create a logical session with `origin=attached` |
| Archive | `/sessions archive S2` hides it; data retained per retention policy; Claude transcripts are never deleted by the bot |
| Workspace | Fixed at session creation (the Claude project directory is derived from the launch cwd); `/cwd` changes the default for future sessions only |

### 6.5 Mapping summary

One Telegram private chat ↔ one chat state (active session pointer + chat defaults). One logical session ↔ exactly one Claude session UUID ↔ one workspace ↔ at most one running job ↔ at most one child PID/PGID at a time. Jobs reference the Telegram update and message IDs that created them, so replies thread back to the originating message.

---

## 7. Command reference

Conventions: commands are lowercase; arguments are space-separated; IDs may be given in short form (`S3`, `j-17`, first 8 hex chars of a Claude UUID). Every command is registered with `setMyCommands` scoped to the owner's chat (`BotCommandScopeChat`). Unknown commands → "Unknown command. /help". Every handler validates arguments strictly and replies with a usage line on error. All replies are HTML-formatted with escaping (Section 9.5).

### 7.1 Required commands

| Command | Semantics | Errors / edge cases |
| --- | --- | --- |
| `/start` | Clears `paused`, sends the menu (compact help + current session/model/effort/workspace line) and registers the command menu. If the bot process is offline no command can reach it; the runbook (Section 18) covers launchd restart. | Idempotent |
| `/help` | Compact command reference (fits one message) | — |
| `/status` | One screen: bot version/uptime, polling health (last update age, 409 seen?), Claude CLI version + auth (cached: `loggedIn`, `authMethod`, `subscriptionType`, cache age), active session (id, name, workspace, mode, model, effort, turns), running/queued jobs, last error (redacted, time) | Answers from memory/DB; < 50 ms |
| `/usage` | Uptime; jobs today/all-time by state; sessions count; bot RSS, sum of child RSS, load average, free memory (from `vm_stat`/`sysctl`); tokens (in/out/cache) summed from job rows (today / 7 days); `total_cost_usd` **labelled** "client-side list-price estimate; not billed on a Max subscription"; rate-limit windows from the last `rate_limit_event` (5-hour and 7-day utilization + reset times, with its timestamp) **labelled** "observed from CLI stream; undocumented field". Any metric that has never been observed prints `unavailable` | Never fabricates numbers |
| `/stop [j-id \| S-id \| all]` | No argument: cancels the running job of the *active* session (and drops its queue); says what was stopped or "nothing is running". `j-17`: cancels that job. `S2`: cancels running + queued jobs of that session. `all`: if more than one job is active → confirmation prompt (inline buttons, 120 s); otherwise immediate. **Never** stops the bot itself | Unknown ID → error; already finished → "j-17 already completed at …" |
| `/processes` | Section 11 output: bot-owned Claude jobs, Claude Code sessions from the cached `claude agents --json`, system summary, cache ages | Read-only; stop affordances only for bot-owned jobs and (Phase 2) background sessions with confirmation |
| `/model [name \| alias] [--default]` | Section 8. Without args: show effective model for the active session and the layers. With a name: validate, probe, apply to the active session (or chat default with `--default`) | Unsupported → prior state preserved, exact reason shown |
| `/effort [low\|medium\|high\|xhigh\|max\|auto] [--default]` | Section 8. `auto` = do not pass `--effort` (CLI/user-settings/model default) | Invalid → usage; unsupported for model → warning + fallback explanation |

### 7.2 Recommended commands (MVP unless marked)

| Command | Semantics |
| --- | --- |
| `/sessions [all]` | List active (and with `all`, archived) sessions: `S3* telegram-bot · ws:telegram-bot · assisted · fable/high · 12 turns · 5 min ago · c6f4f93c`. `*` marks active |
| `/new [name] [--ws <workspace>]` | Create and activate a session (Section 6.4). Name must match `[A-Za-z0-9_-]{1,32}` |
| `/switch <S-id \| name>` | Activate an existing bot session |
| `/resume <claude-uuid>` (Phase 2) | Attach an external Claude session after ownership validation (Section 6.4) |
| `/jobs [n]` | Last n (default 10) jobs with state, session, duration, tokens |
| `/cancel <j-id>` | Cancel a specific queued/running job (same ladder as `/stop`) |
| `/logs <j-id> [lines]` | Bounded (default 40 lines, max 200), redacted tail of the job's event log: tool uses (tool name + ≤120 chars of input), denials, stderr warnings, result preview. Never raw environment, never full transcripts |
| `/cwd [name]` | Show the default workspace and the allowed roots; with a name, set the default workspace for new sessions. Only configured roots are accepted (no free-form paths) |
| `/mode [observe\|assisted\|automation]` | Show or set the permission tier of the active session. `automation` requires `modes.automation_enabled` and a confirmation |
| `/doctor` | Runs read-only diagnostics (Section 18.5) and posts a redacted report; warns that it may take ~30–40 s because `claude auth status` and `claude agents --json` are slow |
| `/aliases`, `/alias set <name> <model>`, `/alias rm <name>` | Custom friendly names → validated model (Section 8.4) |
| `/confirm <token>` / `/deny <token>` | Text fallback for inline confirmation buttons |
| `/pause` | Stop accepting new jobs (running ones continue); `/start` resumes |
| `/shutdown` | Disabled unless `shutdown_command_enabled: true`; with confirmation, graceful exit (launchd `KeepAlive` will restart it unless the owner unloads the agent — explained in the reply) |
| `/run <capability> [args]` (Phase 2) | Allowlisted typed operations (Section 10.3) |
| `/fork` (roadmap) | `--fork-session` into a new logical session |

### 7.3 Plain messages

A non-command text message is a prompt for the active session. If no session exists, one is created in the default workspace with the chat defaults. Edited messages are not delivered (`allowed_updates` excludes `edited_message`). Photos/documents are rejected in the MVP with a short notice ("Attachments are not enabled"); Section 19 covers the later attachment policy. Messages longer than 32 KB are rejected ("too long; put it in a file inside the workspace and reference the path").

### 7.4 Realistic examples

```
Owner:  /start
Bot:    Claude control center · clawdyx 0.1.0 · up 2h14m
        Session S3 "telegram-bot" · ws telegram-bot · observe · fable/high
        /help for commands. Send a message to talk to Claude.

Owner:  What does internal/policy do? Keep it short.
Bot:    ⏳ j-17 running · S3 · fable/high · observe            [Cancel]
        (draft preview streams here while Claude works)
Bot:    internal/policy validates workspace paths, maps permission tiers to CLI flags, …
        — j-17 · 14.2s · 1 turn · in 2.1k / out 310 · fable

Owner:  /model sonnet
Bot:    Validating sonnet… ✅ Model for S3 set to sonnet → claude-sonnet-5-5 (verified on CLI 2.1.292).
        Applies to the next message. Effort stays high.

Owner:  /effort xhigh
Bot:    ✅ Effort for S3 set to xhigh (supported by claude-sonnet-5-5).

Owner:  /model fable
Bot:    ✅ Model for S3 set to fable → claude-fable-5-1 (primary model; the advisor model is unchanged/unused).

Owner:  /model gpt-5
Bot:    ❌ Not changed. "gpt-5" is not a known alias, custom alias, or claude-* model ID. Known: default, best, fable, opus, sonnet, haiku, opus[1m], sonnet[1m], opusplan. /aliases for custom names.

Owner:  /stop
Bot:    ⛔ Cancelled j-18 (S3, 41 s running). Partial output: /logs j-18. The session is still resumable.

Owner:  /stop all          (two jobs running)
Bot:    Stop 2 jobs (j-19 in S3, j-20 in S4)?  [Stop both] [Keep running]   (expires in 120 s)

Owner:  /processes
Bot:    Bot-owned Claude jobs
        • j-19 · S3 telegram-bot · pid 15017 · running 1m12s · fable/high · assisted · 338 MB · 12% CPU
        Claude Code sessions (claude agents --json, 23 s ago)
        • interactive · gujurly-sat-5f · idle · pid 3670 · since 09:12 · ~/projects/gujurly_sat
        • interactive · telegram-bot-agent-claude-6a · busy · pid 10737 · since 09:53 · (this bot's repo)
        • background · none
        System · load 2.9 / 2.6 / 2.8 · free 0.8 GiB of 36 GiB · 860 processes
        Stop a bot job: /cancel j-19

Owner:  /usage
Bot:    Uptime 2h14m · jobs today 9 (7 ok, 1 cancelled, 1 failed) · sessions 4
        Tokens today: in 48.2k · out 9.1k · cache read 612k · cache create 71k
        Cost estimate today: $3.84 (client-side list price; included in Max, not billed)
        Rate limits (observed 3 min ago, undocumented): 5-hour 15% (resets 14:00) · 7-day 45% (resets Oct 18 00:00)
        Memory: bot 21 MB · Claude children 338 MB · free 0.8 GiB

Owner:  (unauthorized user sends anything)
Bot:    (no reply; counter increments; nothing logged about the sender)
```

### 7.5 Command/state transition table

| Command | Pre-state | Post-state | Persisted |
| --- | --- | --- | --- |
| plain text | active session S (or none) | job queued/running for S (S created if none) | job row, session row |
| `/new` | any | new session active | session row, settings.active_session |
| `/switch` | S exists | S active | settings.active_session |
| `/model X` | any | S.model = X (or settings.default_model) after probe | session/settings row, capability cache |
| `/effort L` | any | S.effort = L or NULL(auto) | session/settings row |
| `/mode M` | any | S.mode = M (automation needs confirmation) | session row, audit |
| `/stop`, `/cancel` | job running/queued | job cancelling → cancelled | job row, audit |
| `/cwd W` | W allowed | settings.default_workspace = W | settings |
| `/pause`, `/start` | — | settings.paused | settings |
| confirmation callback | token valid, unused, unexpired, HMAC matches | action executed; token used | confirmations, audit |

---

## 8. Model and effort controls

### 8.1 Facts the design relies on (verified)

- `fable` is a **primary model alias** in 2.1.292: `--model fable` → `claude-fable-5-1` (1 M context, 64 k max output). Also `best`, `default`, `opus`, `sonnet`, `haiku`, `opus[1m]`, `sonnet[1m]`, `opusplan` are documented aliases; `fable[1m]` was accepted and resolved to `claude-fable-5-1`.
- `--advisor` is a hidden, separate flag (advisor tool model). The bot does not pass it; `/model` never touches it; the reply states "primary model" explicitly. A future `/advisor` command could expose it (roadmap).
- Model precedence (docs): `--model` > `ANTHROPIC_MODEL` > settings `model` > org default > account default (Opus 5.5 on Max). The owner's user settings set `model: "fable"`, so an unset model would still be Fable today; the bot passes `--model` explicitly anyway.
- Resumed sessions keep their transcript model unless `--model` is passed (docs; verified that passing it switches).
- Effort levels: `low, medium, high, xhigh, max`. Docs: Fable 5.1/5, Opus 5.5, Sonnet 5.5, Haiku 5.5, Opus 5, Sonnet 5, Opus 4.8, Opus 4.7 support all five; Opus 4.6 and Sonnet 4.6 lack `xhigh`; unsupported levels fall back to the highest supported level at or below. Defaults: `medium` for Opus 5.5/Sonnet 5.5/Haiku 5.5, `xhigh` for Opus 4.7, `high` otherwise; the owner's `modelSettings` override these per model when user settings are loaded. `ultracode` is a workflow setting, not an effort level (not exposed).
- The CLI **ignores** unknown `--effort` values with a stderr warning and runs anyway; nothing in the output reports the effective effort.

### 8.2 `/model` behaviour

1. Parse `name` (trim, lowercase for aliases; keep case for full IDs). Resolve custom aliases first (`/alias`), then built-in aliases, then full IDs matching `^claude-[a-z0-9.-]+(\[1m\])?$`. Anything else → rejected without a process start.
2. Probe unless the capability cache has `model:<name>` verified for the **current CLI version** within 7 days: run `claude -p --model <name> --max-turns 1 --tools "" --no-session-persistence --output-format json --permission-mode dontAsk --permission-prompts none` with stdin `Reply with exactly: OK`, 60 s timeout. Success = `is_error: false` and `modelUsage` has exactly one key → canonical ID. Failure with `api_error_status: 404` or stderr `[claude-code:unrecognized_model]` → "not available to this account/CLI". Cost: ≈ 12–15 k cache-creation tokens (list-equivalent ≈ $0.2–0.3, included in Max). The reply says "Validating…" first.
3. Apply to scope: default scope is the **active session**; `--default` sets the chat default for new sessions. `--session <S-id>` applies to another session.
4. Confirm only after the probe succeeded: "✅ Model for S3 set to fable → claude-fable-5-1 (verified on CLI 2.1.292)". The next job's `system/init.model` is cross-checked; a mismatch reverts the setting and reports it.
5. Effect timing: applies to the next job (flags are per process). Running jobs are unaffected (stated in the reply).

### 8.3 `/effort` behaviour

Validate against `{low, medium, high, xhigh, max, auto}`. Check the docs-derived support table for the session's canonical model; if the level is unsupported, apply anyway (the CLI falls back) but say so: "xhigh is not supported by claude-sonnet-4-6; Claude Code will run it as high". `auto` stores NULL → no `--effort` flag. Each job records the effort it passed; `/status` and job footers show it (`fable/high`, `fable/auto→cli-default`). If a job's stderr contains `Unknown --effort value`, the job footer shows a warning and the setting is reset to `auto`.

### 8.4 Aliases

`/alias set fast haiku` stores `fast → haiku` after the same validation/probe as `/model`. Names: `[a-z0-9_-]{1,24}`, must not shadow built-in aliases or effort levels. `/aliases` lists name → target → canonical ID → verified date. Aliases live in SQLite, not in the owner's Claude settings.

### 8.5 Capability discovery and cache invalidation

The bot never hardcodes model names as permanently valid. Discovery sources, in order: built-in alias list (from docs, with the doc date), the probe above, and the `system/init.model`/`modelUsage` echoes of real jobs. The cache is keyed by `claude --version`; a version change (checked hourly and at startup) invalidates all entries and triggers one smoke probe with the chat default model (Section 18.6). The bot may *display* the `model` and `modelSettings` keys of `~/.claude/settings.json` (read-only, documented file) in `/model` as "CLI default when unset"; it never writes that file and never reads other keys.

---

## 9. Chat UX, streaming, and long-running jobs

### 9.1 Acknowledgement and job identity

Within 500 ms of receiving a prompt the bot replies (as a reply to the owner's message): `⏳ j-17 queued (position 2) · S3 · fable/high · observe` or `running`, with an inline **Cancel** button whose callback data is `cj:17:<8-char nonce>`. State changes edit this message (`running`, `cancelling`, `done ✓ 14 s`). Nothing is sent to Telegram before the job row is persisted.

### 9.2 Streaming

Primary: Telegram **`sendMessageDraft`** (Bot API 9.3+, all bots since 9.5; private chats only). The bot coalesces `text_delta` events and sends a draft at most once per second (`draft_id` = job number, `can_stop: true`). Telegram renders an ephemeral preview (30 s) and the Stop button yields a `stopped_message_generation` update, which maps to cancellation. The final text is always sent with `sendMessage` (drafts do not persist).

Fallback (if the library/API rejects drafts): a single progress message edited at most every 3 s with the last ≤ 3,000 chars of streamed text plus a tool activity line; `sendChatAction typing` every 4 s while the job runs.

Tool activity (from `assistant` events with `tool_use` blocks and `user` tool results): `🔧 Read internal/policy/paths.go`, `🔧 Bash go test ./... (12 s)`, `🚫 denied: Bash touch x.txt`. Shown in the progress line and stored in `JobEvent` (bounded).

### 9.3 Final output

Markdown from Claude is converted to Telegram HTML (bold/italic/code/pre with language, links kept only if `http(s)`), escaped with `html.EscapeString` for everything else, and split into ≤ 4,000-char chunks on paragraph or line boundaries; open code fences are closed at a chunk end and reopened at the next. If Telegram rejects a chunk with `can't parse entities`, the chunk is resent as plain text. Output caps: 1 MiB captured per job, 200 KB sent (then "… truncated; full output in /logs j-17 (last 200 lines)" ). Footer: `— j-17 · 14.2 s · 3 turns · in 2.1k / out 310 · cache r 61k · fable/high`. Cost is shown only in `/usage` with its label. Local paths are shown only if inside the session's workspace; other absolute paths are replaced with `<outside workspace>`.

### 9.4 Limits and backpressure

| Parameter | Default | Config key |
| --- | --- | --- |
| Job timeout | 15 min (max 2 h) | `claude.job_timeout` |
| Max turns per job | 60 | `claude.max_turns` |
| Budget cap (estimate) | off (`0`); if set, ≥ $2 | `claude.max_budget_usd` |
| Concurrent jobs | 2 (cap 4) | `claude.max_concurrent_jobs` |
| Queue per session | 5 | `claude.max_queue_per_session` |
| Captured output | 1 MiB | `claude.max_output_bytes` |
| Child RSS watchdog | 2 GiB → cancel with reason | `claude.max_child_rss_mb` |
| Telegram sends | 1/s per chat, burst 3; edits ≤ 1 per 3 s per message; drafts ≤ 1/s | fixed |
| Background wait at exit | 60 s (`CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=60000`) | `claude.bg_wait_ceiling_ms` |

Queue full → "Queue for S3 is full (5). /stop to clear or wait." Global slots busy → jobs stay `queued` with position shown.

### 9.5 Formatting safety

HTML parse mode only (MarkdownV2 escaping is context-sensitive and the library's `EscapeMarkdown` is incomplete). Everything originating from Claude, tool output, file names, or the CLI is escaped; only bot-generated tags are emitted. `callback_data` ≤ 64 bytes and treated as untrusted (Section 14.4).

### 9.6 Telegram connectivity failures and restarts

- Polling errors: exponential backoff 1 s → 30 s; `/status` shows "last successful poll N s ago". Running jobs continue; their results are queued in the outbound sender (bounded 200 messages, oldest dropped with a log line) and delivered when connectivity returns.
- Re-delivered updates after a crash are ignored via `processed_updates` and the UNIQUE `tg_update_id` on jobs; the stored `last_update_id` seeds `WithInitialOffset`.
- Restart with jobs running: Section 6.3 (`interrupted`; no automatic re-run).

### 9.7 Attachments (not in MVP)

Enable only after Section 14 review: accepted types `text/*`, `application/json`, images ≤ 5 MB, documents ≤ 10 MB; stored under `<state_dir>/tmp/<job>/` (0700), deleted after 24 h or at job end; referenced in the prompt by path only; never executed; contents treated as untrusted data.

---

## 10. Computer-control scope and the capability registry

### 10.1 Staged capability plan

| Stage | Capability | Mechanism | MVP |
| --- | --- | --- | --- |
| 1 Read-only visibility | `/status`, `/processes`, `/usage`, `/doctor`, `/logs` | Local state, `ps`, `vm_stat`, `sysctl`, cached `claude agents --json`, `claude doctor` | Yes |
| 2 Workspace work | Claude reads/edits inside allowed roots | `observe` / `assisted` tiers via CLI permission flags | Yes |
| 3 Controlled operations | `/run test`, `/run build`, `/run git-status` … | Capability registry with typed handlers and `exec.CommandContext` argv arrays; timeouts; recorded results | Phase 2 |
| 4 Desktop/GUI automation | AppleScript/Shortcuts adapters | Explicit allowlisted scripts only; macOS Automation/Accessibility permissions | Optional Phase 4 |

### 10.2 Execution rules (all tiers)

- Launch with `exec.CommandContext`, argv slice, `cmd.Dir` = canonical workspace path, `SysProcAttr{Setpgid: true}`, custom `cmd.Cancel` sending SIGINT to `-pgid`, `cmd.WaitDelay` = 5 s, explicit escalation to SIGTERM/SIGKILL of the group.
- Never `sh -c`; never interpolate Telegram text into a command; prompts go to Claude's stdin only.
- Executable resolution: `claude` from `claude.binary` (absolute path, validated at startup to be a regular executable file; default `/opt/homebrew/bin/claude`); other capabilities use absolute paths from config or `exec.LookPath` against the bot's own fixed PATH.
- Child environment (Section 13.4) is an allowlist; the bot token and any `ANTHROPIC_API_KEY` are never inherited.
- Working directories: only configured roots after `filepath.EvalSymlinks` + `filepath.Clean`; the canonical path must stay inside the canonical root; reject `..`, symlink escapes, and non-directories.

### 10.3 Capability registry (Phase 2)

```go
type Capability struct {
    Name        string            // "test", "build", "git-status"
    Description string
    Tier        Tier              // observe | assisted | automation
    Confirm     bool              // requires an expiring confirmation
    Argv        []string          // fixed program + args; may contain typed placeholders {pkg}
    Args        []ArgSpec         // name, regexp, max length
    Timeout     time.Duration
    MaxOutput   int
}
```

Examples shipped by default (per workspace, in config): `test` → `go test ./...`, `build` → `go build ./...`, `vet` → `go vet ./...`, `git-status` → `git status --short`, `git-log` → `git log --oneline -n {n}` (`n` ∈ 1..50). Output is captured, redacted, truncated, and posted with exit code and duration. No capability may take a free-form path or command string.

### 10.4 Interactive approvals (Phase 3): MCP permission-prompt tool

`claude -p … --permission-prompts host --permission-prompt-tool mcp__claude_tg__approve --mcp-config '{"mcpServers":{"claude_tg":{"command":"/path/clawdyx","args":["mcp-approve","--socket","<state_dir>/approve.sock"]}}}'`. The helper is an MCP stdio server with one tool `approve`; each call carries the pending tool name and input; the helper forwards it over a Unix socket (0600, inside the state dir, no TCP) to the bot, which posts `🔐 Claude wants to run: go generate ./... [Allow] [Deny]` with a 120 s expiry, then returns the decision as the tool result: a JSON string `{"behavior":"allow","updatedInput":<input>}` or `{"behavior":"deny","message":"Owner declined"}` (this is the SDK `canUseTool` contract, documented; the exact MCP tool input field names must be confirmed against the current docs when implementing — `DOCS-ONLY`). Timeout → deny. The job's permission mode stays `acceptEdits`/`dontAsk`; only calls that would otherwise prompt reach the tool.

### 10.5 Desktop GUI automation (evaluated, not recommended for early phases)

`osascript` and `shortcuts` exist on the machine. A LaunchAgent-hosted binary driving AppleScript needs Automation permission per target app and, for UI scripting, Accessibility permission granted to the **bot binary**; TCC prompts for non-bundled binaries launched by launchd are unreliable and may need manual grants in System Settings. The Claude Code CLI itself has no desktop-clicking capability (Claude in Chrome is a separate browser integration and is disabled with `--strict-mcp-config`/`--no-chrome`). If ever added: fixed, reviewed `.scpt`/Shortcut names in config, typed arguments, confirmation per run, and a documented permission grant procedure.

### 10.6 Sensitive actions

| Action class | Policy |
| --- | --- |
| Delete files / bulk overwrite inside workspace | `assisted` tier only; `Bash(rm *)`, `Bash(rmdir *)` denied by scoped rules (Edit/Write still allowed); bulk operations are Claude's edits under git, visible via `/run git-status` |
| Modify anything outside allowed roots | Denied by the CLI (outside working directories) and by bot policy |
| Kill processes | Only bot-owned jobs (`/cancel`) and Claude background sessions via `claude stop <id>` with confirmation (Phase 2). Never by name match |
| Install/update/uninstall software, system/privacy settings, sudo | Blocked entirely (`Bash(sudo *)`, `Bash(brew install *)`, `Bash(claude update *)` … deny rules; no capability exists) |
| Read credentials, keychains, SSH keys, browser profiles | Blocked: `Read(//Users/<you>/.ssh/**)`, `Read(//Users/<you>/.claude/**)`, `Read(//Users/<you>/.aws/**)`, `Read(//Users/<you>/Library/Keychains/**)`, `Read(//Users/<you>/.config/**)`, `Read(//Users/<you>/Library/Application Support/Google/**)` deny rules in every tier; the bot itself never reads them |
| Send messages/email/publish/purchase | No capability; `Bash(curl *)`, `Bash(wget *)`, `WebFetch`, `WebSearch` denied in observe/assisted; git remotes/push denied (`Bash(git push *)`, `Bash(git remote *)`) |
| Restart/sleep/shutdown computer or service | No capability; `/shutdown` of the bot only when enabled + confirmed |
| Upload file contents externally | Only through the ordinary Claude request flow (prompt + Claude's own tools) |

Confirmations are bound to `(action, canonical args, chat id, token, expiry)` via HMAC with a per-install secret; a stale or mismatched confirmation is rejected and audited.

---

## 11. Process and session monitoring

### 11.1 Sources (in priority order)

1. **Bot job registry** (SQLite + in-memory): authoritative for jobs and their PIDs/PGIDs.
2. **`claude agents --json`** (documented, 10–15 s): interactive and background Claude Code sessions with `pid`, `cwd`, `kind`, `status`, `sessionId`, `name`, `startedAt`; `--all` for completed background sessions. Refreshed by a background goroutine every 60 s; never on the request path; `/processes` prints the cache age and `(stale)` if > 3 min.
3. **`ps -axo pid,ppid,pgid,rss,pcpu,etime,lstart,comm`** (32 ms): resource stats for bot-owned PIDs and the count of other `claude` processes. `pcpu` from `ps` is a lifetime average; for an instantaneous figure sample twice 1 s apart or label it "avg since start". Never print process arguments.
4. **System summary**: `sysctl vm.loadavg`, `vm_stat` (free/inactive pages × 16 KiB), `hw.memsize`. Cheap (≈ 6 ms). gopsutil can replace the parsing if preferred (pure Go on darwin for these calls).
5. Fallback to transcript files: only the existence check in Section 6.4.

### 11.2 Ownership proof before any stop

A process may be signalled only if: its PID is in the bot's registry **and** the live process start time equals the stored `proc_start_unix` (guards PID reuse) **and** it is a `claude` executable. Background sessions are stopped only via `claude stop <id>` after confirmation. Interactive terminal sessions are listed, never signalled.

### 11.3 `/processes` layout

See the example in Section 7.4. Sections: bot-owned jobs → Claude Code sessions (interactive/background, marking ones that belong to bot jobs by `sessionId`) → other `claude` processes count → system summary → cache ages. Paths are shown only when under an allowed root; otherwise the home-relative path is shown for interactive sessions of the owner (single authorized user) but truncated to 60 chars.

---

## 12. Go dependencies and decisions

### 12.1 Telegram library comparison (checked 2026-10-16)

| Library | Latest | Bot API | Deps | Maintenance | Notes |
| --- | --- | --- | --- | --- | --- |
| **`github.com/go-telegram/bot`** | v1.27.0 (2026-09-11) | 10.3 | none (stdlib only), Go ≥ 1.18 | last commit 2026-10-08; 23 open issues | Context-aware `Start(ctx)`, `RegisterHandler` (command/prefix/exact, callback data), middleware, `WithNotAsyncHandlers` (ordered processing), `WithInitialOffset`, `WithAllowedUpdates`, `WithServerURL` (httptest mocks), `WithSkipGetMe`, `WithErrorsHandler`; getUpdates loop honours 429 `retry_after`; other calls return `TooManyRequestsError` (caller retries); has `SendMessageDraft`, `SendChatAction`, `SetMyCommands`, `DeleteWebhook`, `GetWebhookInfo`, `AnswerCallbackQuery`, `EditMessageText` (signatures verified). Caveat: `EscapeMarkdown` is incomplete → use HTML |
| `github.com/go-telegram-bot-api/telegram-bot-api/v5` | v5.5.1 (2021-12-13) | ~5.x | none | last commit 2022-10; 110 open issues | Five years behind the API (no drafts, no newer update types). Rejected |
| `github.com/mymmrac/telego` | v1.12.1 (2026-08-29) | 10.3 | fasthttp, fastjson, sonic, go-json, mock, testify; Go ≥ 1.26.7 | active | Capable but heavy dependency footprint. Not chosen |
| `gopkg.in/telebot.v4` | v4.0.0-beta.10 (2026-06-16) | ~10.1 | viper, go-yaml, testify | beta | Not chosen |

**Decision: `github.com/go-telegram/bot` v1.27.0.** Zero dependencies, current API, testable via `WithServerURL`, ordered handlers available.

### 12.2 Other modules

| Need | Module | Version | Why |
| --- | --- | --- | --- |
| SQLite | `modernc.org/sqlite` | v1.60.1 (2026-09-29), needs `go ≥ 1.26` | CGO-free, darwin/arm64 supported, simplest static binary; slower than C SQLite but the bot's write volume is tiny |
| Single instance | `github.com/gofrs/flock` | v0.13.1 | `flock(2)` wrapper |
| Process/system stats (optional) | `github.com/shirou/gopsutil/v4` | v4.26.9 | Only if `ps`/`vm_stat` parsing proves brittle; otherwise stdlib + `os/exec` |
| UUIDs | `github.com/google/uuid` | v1.6.0 | Pre-generated Claude session IDs |
| `.env` loading | `github.com/joho/godotenv` | v1.5.1 | Optional; the bot can also read env only |
| Tests | `github.com/stretchr/testify` | v1.12.1 | Assertions |
| Log rotation (optional) | `gopkg.in/natefinch/lumberjack.v2` | latest | If launchd stdout files are not enough |

No web framework, no YAML/TOML (config is JSON via `encoding/json`), no CGO (`CGO_ENABLED=0` build), `log/slog` for logging, `os/exec` with `Cmd.Cancel`/`WaitDelay` (Go ≥ 1.20), `syscall.SysProcAttr{Setpgid: true}` and `syscall.Kill(-pgid, sig)` for group signals. `go.mod`: `go 1.26` minimum (forced by modernc.org/sqlite); toolchain `go1.27.x`. Module path: `github.com/Publicthrone547/clawdyX`.

---

## 13. Configuration contract, state paths, logging, LaunchAgent

### 13.1 Files and directories (macOS conventions)

| Purpose | Path | Mode |
| --- | --- | --- |
| Secrets | `~/.config/clawdyx/.env` (only `TELEGRAM_BOT_TOKEN=…`), or the variable in the process environment | 0600; the bot refuses to start if the file is group/world readable |
| Config | `~/.config/clawdyx/config.json` (`CLAWDYX_CONFIG` overrides) | 0600 |
| State | `~/Library/Application Support/clawdyx/` → `state.db` (+ WAL), `bot.lock`, `approve.sock` (Phase 3), `tmp/`, `backups/` | dir 0700 |
| Logs | `~/Library/Logs/clawdyx/bot.log` (JSON lines, rotated at 20 MB × 5) + launchd `stdout.log`/`stderr.log` | 0600 |
| LaunchAgent | `~/Library/LaunchAgents/local.clawdyx.plist` (label `local.clawdyx`) | 0644 |

### 13.2 `.env.example` (committed; real `.env` is git-ignored)

```
# Telegram bot token from @BotFather. Never commit the real value.
TELEGRAM_BOT_TOKEN=123456789:replace-with-real-token
# Optional: override config path
# CLAWDYX_CONFIG=/Users/<you>/.config/clawdyx/config.json
```

### 13.3 `config.json` (defaults shown; `config.example.json` is committed)

```json
{
  "telegram": {
    "owner_user_id": <owner id>,
    "poll_timeout_seconds": 30,
    "allowed_updates": ["message", "callback_query", "stopped_message_generation"]
  },
  "claude": {
    "binary": "/opt/homebrew/bin/claude",
    "default_model": "fable",
    "default_effort": "auto",
    "setting_sources": "user",
    "strict_mcp_config": true,
    "job_timeout": "15m",
    "max_turns": 60,
    "max_budget_usd": 0,
    "max_concurrent_jobs": 2,
    "max_queue_per_session": 5,
    "max_output_bytes": 1048576,
    "max_child_rss_mb": 2048,
    "bg_wait_ceiling_ms": 60000,
    "child_path": "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin",
    "extra_disallowed_tools": []
  },
  "workspaces": [
    {
      "name": "telegram-bot",
      "path": "/Users/<you>/projects/clawdyX",
      "default_mode": "observe",
      "strict_observe": false,
      "allowed_commands": ["Bash(go test *)", "Bash(go build *)", "Bash(go vet *)", "Bash(git status *)", "Bash(git diff *)", "Bash(git log *)"],
      "denied_commands": ["Bash(rm *)", "Bash(rmdir *)", "Bash(sudo *)", "Bash(git push *)", "Bash(git remote *)", "Bash(curl *)", "Bash(wget *)", "Bash(brew *)", "Bash(claude *)"],
      "capabilities": ["test", "build", "vet", "git-status", "git-log"]
    }
  ],
  "default_workspace": "telegram-bot",
  "modes": { "automation_enabled": false, "approval_relay_enabled": false },
  "shutdown_command_enabled": false,
  "logging": { "level": "info", "max_size_mb": 20, "max_files": 5 },
  "retention": { "jobs_days": 90, "job_events_days": 14, "processed_updates_days": 7, "audit_days": 180 }
}
```

Validation (fail closed, exit code 2 with a precise message): `owner_user_id` must be a single positive int64 (no lists, no `*`, no usernames); the token must match `^\d{8,10}:[A-Za-z0-9_-]{35}$`; `claude.binary` must be an absolute path to an executable regular file; each workspace path must exist, be a directory, and be unique after canonicalization; `default_workspace` must exist; modes/effort values must be in their enums; `max_concurrent_jobs` 1..4; durations parse; `denied_commands` must contain at least the defaults for `rm`, `sudo`, `git push` unless `"i_know_what_i_am_doing": true` per workspace.

### 13.4 Child process environment (allowlist)

`HOME`, `USER`, `LOGNAME`, `SHELL=/bin/zsh`, `PATH=<claude.child_path>`, `TMPDIR` (from the bot's env or `/tmp`), `LANG=en_US.UTF-8`, `TERM=dumb`, `CLAUDE_CODE_PRINT_BG_WAIT_CEILING_MS=<n>`, `DISABLE_AUTOUPDATER=1`. Explicitly **never** forwarded: `TELEGRAM_BOT_TOKEN`, `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDECODE`, `CLAUDE_CODE_*` (nested-session markers), `SSH_AUTH_SOCK`, and anything else. If `ANTHROPIC_API_KEY` is present in the bot's own environment, the bot logs a warning at startup ("ignored; the bot uses the local claude.ai login only").

Settings passed per job: `--setting-sources user` (owner's global settings, hooks, plugins; project/local settings of the workspace are **not** loaded so repository-provided hooks or `.mcp.json` servers cannot run — documented `-p` risk), `--strict-mcp-config` (no MCP servers at all, unless Phase 3 adds the approval server), and `--settings '{"permissions":{"disableBypassPermissionsMode":"disable","disableAutoMode":"disable"}}'` (the second key only while automation is disabled).

### 13.5 Logging and retention

`log/slog` JSON: `ts, level, component, event, job_id, session_id, update_id, duration_ms, outcome, error_class`. Never logged: message bodies, prompts, Claude output, tokens, emails, environment dumps, full process arguments. Secrets/PII redaction (Section 14.5) is applied by a wrapping `io.Writer` to every sink. Levels: `info` for lifecycle, `debug` (off by default) for event types only. Retention per config; `clawdyx backup` runs `VACUUM INTO backups/state-<date>.db` and prunes to 7 backups.

### 13.6 LaunchAgent (template generated by `clawdyx service install`; nothing is installed during analysis)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>local.clawdyx</string>
  <key>ProgramArguments</key><array>
    <string>/Users/<you>/go/bin/clawdyx</string><string>run</string>
  </array>
  <key>EnvironmentVariables</key><dict>
    <key>PATH</key><string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
    <key>HOME</key><string>/Users/<you></string>
    <key>CLAWDYX_CONFIG</key><string>/Users/<you>/.config/clawdyx/config.json</string>
  </dict>
  <key>WorkingDirectory</key><string>/Users/<you></string>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><dict><key>SuccessfulExit</key><false/></dict>
  <key>ThrottleInterval</key><integer>10</integer>
  <key>ExitTimeOut</key><integer>30</integer>
  <key>ProcessType</key><string>Interactive</string>
  <key>LimitLoadToSessionType</key><string>Aqua</string>
  <key>StandardOutPath</key><string>/Users/<you>/Library/Logs/clawdyx/stdout.log</string>
  <key>StandardErrorPath</key><string>/Users/<you>/Library/Logs/clawdyx/stderr.log</string>
</dict></plist>
```

Implications: runs only in the Aqua (logged-in GUI) session, so keychain-backed claude.ai auth is available; starts at login; restarts on crash (not on clean exit, so `/shutdown` really stops it until the next login or `launchctl kickstart`); the token is **not** placed in the plist (read from the 0600 `.env`). `ProcessType Interactive` avoids background-QoS throttling of the child `claude` processes. TCC: keep workspaces outside `~/Desktop`, `~/Documents`, `~/Downloads` (protected folders need Files-and-Folders grants for a launchd-launched binary).

---

## 14. Security

### 14.1 Threat model and mitigations

| Threat | Impact | Mitigation |
| --- | --- | --- |
| Stolen Telegram bot token | Attacker can read/send as the bot and hijack updates via `setWebhook` | Owner-only gate (attacker's messages are dropped silently even with the token); startup `getWebhookInfo`/`deleteWebhook` with alert if a webhook was set; 409 detection; token rotation runbook (Section 18.7); token only in 0600 `.env`, never logged or echoed |
| Unauthorized Telegram user | Any side effect | Gate on `from.id` for every update type before any lookup; private chat only; `chat.id == from.id`; silent drop; counter only |
| Forged/stale callback or confirmation | Replay of a sensitive action | Gate applies to `callback_query.from`; HMAC-bound, one-time, 120 s tokens; action+args hash compared; audit |
| Prompt injection via repo files, tool output, web content, attachments | Claude attempts dangerous actions | CLI permission tiers (deny rules evaluated before any mode; no bypass; outside-workspace access denied); `WebFetch`/`WebSearch`/`curl`/`wget` denied in observe/assisted; project settings/hooks/MCP of the workspace not loaded (`--setting-sources user`, `--strict-mcp-config`); Claude output is displayed, never executed by the bot |
| Path traversal / symlink escape | Access outside roots | Canonicalize (`EvalSymlinks`) and prefix-check every path; only configured roots are selectable; Claude's own file tools are confined by the CLI |
| Shell argument injection | Arbitrary command execution | No `sh -c`; argv arrays; prompts via stdin; capabilities with typed, regexp-validated args |
| Leaked environment variable | Secret exposure to Claude/tools | Allowlisted child env; `ps` with `e` not used; `/doctor` prints variable names only |
| Secret-bearing logs/replies | Exposure | Redaction writer on all sinks; `claude auth status` fields `email/orgId/orgName` stripped; `rate_limit_event` passes through only numeric windows |
| Duplicate pollers | Lost/duplicated updates | `flock`; 409 handling; idempotent job keys |
| Runaway child process | Resource exhaustion | Job timeout, max turns, optional budget, RSS watchdog, process-group kill, `WaitDelay`, concurrency cap |
| Overloaded machine | Latency, failures | Concurrency cap 2 (max 4), queue limits, refreshers skip under load |
| Accidental destructive operation | Data loss | observe default; `rm/rmdir/sudo/git push/brew/claude` denied in assisted; confirmations for `/stop all`, mode escalation, background-session stop |
| Invalid/expired Claude login | Jobs fail | Startup + periodic `auth status`; failures surface as `/doctor` guidance ("run `claude auth login` in Terminal"); never falls back to an API key |
| Future CLI change (`--bare` default, flag renames) | Silent auth/behaviour change | Version detection, capability cache invalidation, smoke probe on version change, tolerant parsing with unknown-event logging |

### 14.2 Authorization gate specification

```
func authorize(u *models.Update) (ok bool):
  switch:
    u.Message != nil:            from = u.Message.From; chat = u.Message.Chat; reject if u.Message.SenderChat != nil || from.IsBot
    u.CallbackQuery != nil:      from = u.CallbackQuery.From; chat = u.CallbackQuery.Message.Chat (reject if message absent/inaccessible)
    u.StoppedMessageGeneration != nil: from = nil; chat = u.StoppedMessageGeneration.Chat  (accept only if chat.ID == owner && chat.Type == private)
    default:                     reject
  ok = from != nil && from.ID == cfg.OwnerID && chat.Type == "private" && chat.ID == cfg.OwnerID
  on reject: metrics.unauthorized++ ; no reply, no log line containing identity or text
```

Registered as the first middleware; command handlers never see unauthorized updates. `allowed_updates` restricts delivery to the three types above. Tests (Section 16) cover each update type, group/supergroup/channel contexts with the owner as sender, bots, `sender_chat`, and mismatched `chat.id`.

### 14.3 Allowed capabilities and workspaces

Only configured roots; only registered capabilities; only two tiers enabled by default. Escalation to `automation` or approval relay requires config **and** a per-session confirmation.

### 14.4 Callback data

Format `<kind>:<id>:<nonce>` ≤ 64 bytes; the bot looks up `(kind, id)` in memory/DB and compares the nonce; unknown or expired → `answerCallbackQuery("expired")` and no action.

### 14.5 Redaction rules (applied to logs, Telegram replies, `/doctor`, `/logs`)

Patterns: Telegram tokens `\b\d{8,10}:[A-Za-z0-9_-]{35}\b`; `sk-ant-[A-Za-z0-9_-]+`; `Bearer [A-Za-z0-9._-]+`; `(?i)(api[_-]?key|token|secret|password|authorization)\s*[=:]\s*\S+` → `<redacted>`; emails; `AKIA[0-9A-Z]{16}`; UUID-like `orgId`; paths under `~/.ssh`, `~/.aws`, `~/.claude/.credentials.json`, `Library/Keychains`. Replacement keeps the key name and the first 2 characters of the value for debugging.

### 14.6 Data classification

| Data | Stored | Logged | Sent to Telegram | Retention |
| --- | --- | --- | --- | --- |
| Bot token | `.env` 0600 only | never | never | until rotated |
| Owner user ID | config | yes (as config) | yes | — |
| Prompts | SHA-256 + optional 80-char preview | never | echoed only as the owner's own message | preview 90 days |
| Claude results | 2 KB preview per job; full text is streamed, not stored | never | yes (chunked, redacted paths) | 90 days |
| Tool activity | bounded `JobEvent` (≤1 KB each, redacted) | event type only | `/logs`, progress line | 14 days |
| Session IDs, workspace paths, model/effort | yes | yes | truncated IDs; paths only inside roots | 90 days / archived |
| Usage/cost estimates, rate-limit windows | per job | no | `/usage` with labels | 90 days |
| Auth status | memory cache (loggedIn, method, subscription) | `loggedIn` only | `/status`, `/doctor` (redacted) | 15 min |
| Unauthorized updates | counter | count only | count only (`/status`) | — |
| Claude transcripts | never copied; owned by Claude Code | never | never | Claude Code `cleanupPeriodDays` |

### 14.7 Risk register (severity × likelihood)

| # | Risk | Sev | Lik | Response |
| --- | --- | --- | --- | --- |
| 1 | `-p` switches to `--bare` by default in a future release and loses OAuth | High | Medium (docs announce it) | Version-change smoke probe; `/doctor` explains; pin cask version in the runbook until the bot is updated to pass the documented opt-out flag |
| 2 | Concurrent resume corrupts a session | High | Low (bot serializes) | Per-session mutex + tests |
| 3 | Stale cached process view (10–15 s refresh cost) | Low | High | Show cache age; refresh on demand with a notice |
| 4 | `acceptEdits` auto-approves `rm` inside the workspace | Medium | Medium | Scoped deny rules; git-tracked workspaces; observe default |
| 5 | Hook/plugin failures under launchd (iTerm2 status hook) | Low | Medium | Hook failures are non-fatal; `claude.setting_sources` can drop user settings; documented |
| 6 | Telegram draft API behaviour changes | Low | Low | Fallback to edited progress message |
| 7 | Undocumented `rate_limit_event` disappears | Low | Medium | `/usage` shows `unavailable` |
| 8 | TCC blocks reads in protected folders | Medium | Medium | Keep roots under `~/projects`; document grants |
| 9 | Bot restart leaves orphan `claude` processes | Medium | Low | Registry + start-time ownership check at startup; process groups |
| 10 | Token leak | High | Low | Owner-only gate limits blast radius; rotation runbook |

---

## 15. Performance targets and benchmarks

| Metric | Target | Basis |
| --- | --- | --- |
| Update receipt → acknowledgement sent (in-bot, excluding Telegram latency) | p50 < 150 ms, p95 < 500 ms | DB insert + one API call |
| `/status`, `/jobs`, `/sessions`, `/model`, `/effort` handler latency | < 50 ms | local state only |
| `/processes` handler latency | < 300 ms (cached agents list + one `ps` call ≈ 32 ms) | measured `ps` 0.032 s; `claude agents --json` 10–15 s is never on the path |
| `/doctor` | ≤ 45 s with progress notice | `claude auth status` 19–36 s + `claude doctor` 0.8 s + `agents --json` 10–15 s run concurrently |
| Job start → first streamed token | p50 < 4 s, p95 < 8 s for simple prompts | measured process overhead 1.3–2.9 s + API TTFT 2–3 s |
| Idle memory (bot) | < 40 MB RSS | Go + SQLite |
| Per running job | ≈ 300–350 MB RSS for text-only jobs (measured peak 343 MB); more with tools | cap via `max_child_rss_mb` |
| Max concurrent jobs | 2 default, 4 hard cap | 36 GiB machine; keeps < 2 GiB for jobs |
| Message sizes | ≤ 4,000 chars per Telegram message; ≤ 200 KB per job delivered | Telegram 4,096 limit |
| Restart recovery | polling resumes < 5 s after start; zero duplicate prompt executions | idempotent keys |

Benchmarks and stress tests (local, no external targets): `go test -bench` for chunking/escaping/redaction; a fake `claude` binary (`testdata/fakeclaude`, a small Go program emitting canned stream-json with configurable delays, exit codes, and signal behaviour) for job throughput, cancellation ladders, and 50-concurrent-update ordering; a fake Telegram server (`httptest` + `WithServerURL`) for 429/409/timeouts.

---

## 16. Testing strategy and acceptance criteria

### 16.1 Test matrix

| Layer | What | How |
| --- | --- | --- |
| Unit: gate | Every update type × sender (owner/other/bot/sender_chat) × chat type (private/group/supergroup/channel) × chat.id mismatch | table tests; assert no handler call and no log line |
| Unit: config | missing token, bad token format, list of IDs, `*`, username, unreadable perms, bad binary, nonexistent workspace, symlinked workspace | fail-closed exit codes |
| Unit: policy | canonical path checks, `..`, symlink escape, non-directory, tier → flags mapping, deny-list minimums | table tests |
| Unit: argv/env builder | stdin prompt, no positional args, variadic flags, env allowlist (no `ANTHROPIC_API_KEY`, no `CLAUDECODE`), `--settings` JSON | golden tests |
| Unit: stream parser | all event types from Appendix B, unknown events, truncated lines, `permission_denied`, `rate_limit_event`, `result` with `is_error`, cumulative `modelUsage` vs per-message usage | fixtures captured on 2.1.292 |
| Unit: render | HTML escaping, chunking at 4,000 with fence reopen, markdown→HTML, path hiding, redaction patterns | golden tests |
| Unit: model/effort | alias resolution, custom aliases, support table, `auto`, stderr warning detection, probe result mapping (404) | table tests |
| Unit: jobs | state machine, queue limits, per-session mutex, global semaphore, cancel ladder timing, timeout, RSS watchdog | fake clock + fakeclaude |
| Integration: fakeclaude | end-to-end prompt → events → Telegram messages; SIGINT/SIGTERM/SIGKILL exits 0/143/137; orphan check; restart with running job → `interrupted`; re-delivered update → no duplicate | `go test ./... -tags integration` |
| Integration: fake Telegram | 409 conflict, 429 retry_after, 5xx, `can't parse entities` fallback, draft unsupported fallback | httptest |
| Failure injection | missing binary, auth status `loggedIn:false`, malformed JSON line, DB locked/read-only, disk full on log | assertions on user-facing text (no secrets) |
| Live (manual, owner's machine) | real `claude` probes behind `-tags live` and `CLAWDYX_LIVE=1`: resume continuity, model switch, effort warning, cancel | documented in `docs/TESTING.md` |
| Security tests | path traversal, symlink escape, argv injection strings, overlong output (10 MB), concurrent sessions isolation, unauthorized callback with valid token | `internal/policy`, `internal/jobs` |
| Lint/static | `go vet`, `staticcheck` (install locally), `gofmt`, `govulncheck` | Makefile targets |

### 16.2 Acceptance criteria mapping (brief §13)

| # | Criterion | Verified by |
| --- | --- | --- |
| 1 | Only `<owner id>` in a private chat gets responses; others cause no side effect | gate unit tests + manual test from a second account |
| 2 | Callbacks and non-text updates are gated | gate tests for `callback_query`, `stopped_message_generation`, others rejected |
| 3 | `/start`, `/usage`, `/stop`, `/processes`, `/model`, `/effort` behave as documented incl. bad args | command handler tests with golden replies |
| 4 | `/model fable` selects the verified primary model; never silently sets an advisor | probe test + argv golden (no `--advisor`) |
| 5 | Model/effort state is accurate in `/status`, jobs, resumes, defaults | state tests + live test |
| 6 | Multiple messages during a long task don't corrupt state or duplicate jobs; cancellation cleans up | fakeclaude integration + live cancel test |
| 7 | `/processes` distinguishes sessions/jobs/owned/unrelated; never signals unrelated processes | ownership-proof tests |
| 8 | Missing CLI, expired login, malformed output, Telegram timeouts/429, job timeouts, restarts, DB failures → safe messages | failure-injection suite |
| 9 | No credentials in logs/replies/commits/snapshots | redaction tests + `git-secrets`-style grep in `make check` |
| 10 | Stale/mismatched confirmations rejected; high-risk actions blocked | confirmation tests |
| 11 | Graceful shutdown; no duplicate poller; owner can restart via launchd | lock tests + runbook |
| 12 | Traversal, symlink, injection, overlong output, isolation, unauthorized callback | security tests |

### 16.3 Local verification commands

```
make build          # CGO_ENABLED=0 go build -o bin/clawdyx ./cmd/clawdyx
make test           # go test ./...
make test-integration  # go test -tags integration ./...
make lint           # gofmt -l . ; go vet ./... ; staticcheck ./... ; govulncheck ./...
make check-secrets  # grep -rnE '[0-9]{8,10}:[A-Za-z0-9_-]{35}|sk-ant-' --exclude-dir=.git . && exit 1 || true
bin/clawdyx check # offline config validation + binary/auth/version checks (prints redacted report)
```

---

## 17. Implementation phases, files, and local Git workflow

### 17.1 Phase 0 — bootstrap (first commit)

Files: `go.mod` (`module github.com/Publicthrone547/clawdyX`, `go 1.26`), `.gitignore`, `README.md`, `Makefile`, `config.example.json`, `.env.example`, `docs/` (this file), `cmd/clawdyx/main.go` (skeleton). Commit: `chore: bootstrap clawdyx module and docs`.

### 17.2 Phase 1 — MVP (milestone commits in this order)

1. `internal/config` + tests — `feat(config): fail-closed configuration`
2. `internal/observability` (slog, redaction) + tests
3. `internal/store` (schema v1, migrations, retention) + tests
4. `internal/policy` (paths, tiers → flags, deny-list defaults) + tests
5. `internal/agent` (argv/env builder, process group runner, stream parser, probes, `testdata/fakeclaude`) + tests
6. `internal/sessions`, `internal/jobs` (queues, state machine, cancel ladder, recovery) + tests
7. `internal/telegram` (poller, gate, router, sender, drafts/edits, chunking) + `internal/render` + tests
8. Commands: `/start /help /status /sessions /new /switch /model /effort /stop /cancel /jobs /usage /processes /doctor /pause`
9. `internal/processes` (registry, `ps`, cached `agents --json`, system summary)
10. `cmd/clawdyx` subcommands: `run`, `check`, `service install|uninstall|status`, `backup`
11. `docs/RUNBOOK.md`, `docs/TESTING.md`; tag `v0.1.0`

### 17.3 Phase 2 — workspace and safety features

`/cwd`, `/mode`, `/logs`, `/aliases`, confirmations (`/confirm`, `/deny`, inline), `/resume <external>`, `/run` capabilities, background-session stop via `claude stop` with confirmation, retention/backup jobs, `/shutdown` (opt-in).

### 17.4 Local Git workflow (mandatory)

- The directory is not a repository: run `git init -b main` (never overwrite an existing `.git`; if one appears later, inspect `git status`/`git log` first and preserve history; move an existing primary branch to `main` with `git branch -m <old> main` only if that does not rewrite history).
- Before the first commit create and review `.gitignore`: `.env`, `*.env`, `*.token`, `config.json` (real), `*.db`, `*.db-wal`, `*.db-shm`, `bin/`, `dist/`, `coverage.out`, `.DS_Store`, `*.log`, `tmp/`, `.idea/`, `.vscode/` (keep `config.example.json`, `.env.example`).
- Before every commit: `git status`, `git diff --cached`, `make check-secrets`. Commit messages: Conventional Commits; include the attribution trailer required by the session.
- Never: `git remote add`, `git push`, `gh auth`, GitHub API calls, publishing. The owner adds the remote and pushes manually.

### 17.5 Phase 3+ — see roadmap (Section 19)

---

## 18. Operational runbook

### 18.1 One-time setup (owner)

1. In Telegram, open @BotFather → `/newbot` → copy the token. Optionally `/setprivacy` is irrelevant (private chat only). Do **not** paste the token into chats, issues, or config.json.
2. `mkdir -p ~/.config/clawdyx && printf 'TELEGRAM_BOT_TOKEN=%s\n' '<token>' > ~/.config/clawdyx/.env && chmod 600 ~/.config/clawdyx/.env`
3. `cp config.example.json ~/.config/clawdyx/config.json`; edit workspaces; `chmod 600` it.
4. `make build && cp bin/clawdyx ~/go/bin/` (or `go install ./cmd/clawdyx`).
5. `clawdyx check` → prints: config OK, binary `/opt/homebrew/bin/claude` 2.1.292, auth `loggedIn=true method=claude.ai subscription=max`, workspaces OK, Telegram `getMe` OK (username), webhook empty.
6. Send `/start` to the bot from the owner account.

### 18.2 Run manually

`clawdyx run` (foreground, logs to stderr and the log file). Stop with Ctrl-C (graceful).

### 18.3 Autostart (LaunchAgent)

`clawdyx service install` writes the plist (Section 13.6) and runs `launchctl bootstrap gui/501 ~/Library/LaunchAgents/local.clawdyx.plist`. Inspect: `launchctl print gui/501/local.clawdyx | head -40`; restart: `launchctl kickstart -k gui/501/local.clawdyx`; stop/uninstall: `launchctl bootout gui/501/local.clawdyx` then `clawdyx service uninstall` (removes the plist; state kept unless `--purge-state`). Logs: `~/Library/Logs/clawdyx/`.

### 18.4 Status and logs

`/status`, `/doctor` in Telegram; `tail -f ~/Library/Logs/clawdyx/bot.log | jq`; `launchctl print` for launchd state.

### 18.5 `/doctor` and `clawdyx check` contents

CLI path/version/commit (from `claude doctor`), auth (`loggedIn`, `authMethod`, `subscriptionType`, cache age; email/org redacted), Telegram polling health and `getMe`, writable state dir and DB integrity (`PRAGMA quick_check`), lock status, child-process smoke (optional `--probe`: one `--max-turns 1` call), capability cache summary, environment variable **names** passed to children, launchd status if installed, last 3 errors (redacted).

### 18.6 Updating

- Claude Code: `brew upgrade --cask claude-code` (owner, in Terminal). The bot detects the new version within an hour (or at restart), invalidates the capability cache, runs one smoke probe, and posts the outcome. If the probe fails with an auth error, the runbook says: check `claude -p` behaviour for the bare-mode default change and update the bot.
- Bot: `git pull` is the owner's job after they add a remote; locally `make build && launchctl kickstart -k gui/501/local.clawdyx`.
- Go: `brew upgrade go`; rebuild.

### 18.7 Rotate or revoke the Telegram token

1. @BotFather → `/revoke` (or `/token`) → new token.
2. Update `~/.config/clawdyx/.env` (0600).
3. `launchctl kickstart -k gui/501/local.clawdyx`.
4. Verify `/status` and that `/doctor` reports `webhook: none` (an attacker with the old token could have set one; the bot deletes it at startup and alerts).
5. Review `unauthorized updates` counter and audit log for the exposure window.

### 18.8 Back up and restore state

`clawdyx backup` (VACUUM INTO `backups/`); restore by stopping the agent, copying the backup over `state.db` (remove `-wal`/`-shm`), starting again. Claude transcripts are not part of the backup (Claude Code owns them).

### 18.9 Troubleshoot local Claude auth

`/doctor` says `loggedIn=false` or jobs fail with an auth error → in Terminal run `claude auth status`; if not logged in, `claude auth login` (interactive, owner only). The bot never logs in, never uses API keys, never reads the keychain.

### 18.10 Uninstall / revert

`launchctl bootout gui/501/local.clawdyx`; `clawdyx service uninstall --purge-state` (deletes state dir, logs, config on request); delete `~/.config/clawdyx/.env`; revoke the token in @BotFather. Claude Code itself is untouched.

---

## 19. Prioritized roadmap

| Priority | Item | Value | Notes |
| --- | --- | --- | --- |
| MVP | Sections 17.1–17.2 | Core control plane | — |
| High | Confirmations, `/mode`, `/cwd`, `/logs`, aliases, `/run` capabilities | Safe workspace automation | Phase 2 |
| High | `sendMessageDraft` streaming polish, Stop button | UX | verify library support |
| Medium | Approval relay via MCP permission-prompt tool | Lets assisted mode ask instead of deny | Phase 3; contract check |
| Medium | Background-session management (`claude stop/logs` after confirmation) | Visibility → control | research-preview CLI surface |
| Medium | Warm sessions (Option B) behind a flag | −1.5–3 s per turn | undocumented control protocol; feature-detect capabilities |
| Low | `/advisor` (expose `--advisor` separately from `/model`) | Power users | hidden flag |
| Low | Attachments with retention policy | Convenience | needs privacy review |
| Low | Automation tier (`--permission-mode auto`) | Fewer denials | classifier-governed; opt-in |
| Optional | Desktop automation adapters | Only if truly needed | TCC permissions, fragile |

---

## 20. Open questions (defaults chosen; owner may override)

1. Default tier for new sessions is `observe`; switch to `assisted` per session with `/mode assisted`. (Alternative: make `assisted` the default for the `telegram-bot` workspace once the deny list is reviewed.)
2. The owner's user-level hooks run inside every job (`--setting-sources user`). If the iTerm2 status hook misbehaves under launchd, set `claude.setting_sources` to `""`/`project` in config; the bot then passes `--model` and effort explicitly, which it already does.
3. Whether to show cost estimates at all on a Max subscription (default: only in `/usage`, labelled).
4. Workspace list: only this repository is configured by default; add others (e.g. `~/projects/gujurly_sat`) explicitly.

---

## Appendix A: Evidence log (redacted, 2026-10-16)

### A.1 What the analysis executed on this machine

Read-only: `sw_vers`, `uname`, `sysctl`, `go env/version`, `ls`, `command -v`, `launchctl getenv/print/list`, `man launchd.plist`, `claude --help`, `claude <sub> --help`, `claude --version`, `claude auth status`, `claude agents --json [--all]`, `claude doctor`, `claude daemon status`, `claude auto-mode config/defaults`, `jq` on `~/.claude/settings.json` (env values redacted) and on selected non-secret keys of `~/.claude.json`, `ps`, `vm_stat`, `top -l 1`.

Bounded capability tests (≈ 45 `claude -p` calls with trivial prompts from scratch directories under the session scratchpad; cost included in the Max subscription; each created a transcript under `~/.claude/projects/-private-tmp-claude-501-…-scratchpad-cwd-test*/` and one plan file under `~/.claude/plans/` from the plan-mode test). Nothing in the owner's repository, settings, credentials, services, or other processes was modified. The owner may remove the test transcripts with `claude purge <scratch path> --dry-run` then without `--dry-run`.

### A.2 Key observations (abridged)

```
claude --version                 → 2.1.292 (Claude Code)
claude auth status               → {"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty",
                                     "subscriptionType":"max","configDirectory":"/Users/<you>/.claude",
                                     "projectsDirectory":"/Users/<you>/.claude/projects", "email":"<redacted>",
                                     "orgId":"<redacted>","orgName":"<redacted>"}
claude agents --json             → [{"pid":3670,"cwd":"/Users/<you>/projects/gujurly_sat","kind":"interactive",
                                     "startedAt":1792139124801,"sessionId":"<uuid>","name":"gujurly-sat-5f","status":"idle"}, …]
claude agents                    → "'claude agents' requires an interactive terminal (stdout is not a TTY) — use 'claude agents --json'"
claude -p --effort bogus "x"     → stderr: Warning: Unknown --effort value 'bogus' — ignoring it and using the default effort.
                                     Valid values: low, medium, high, xhigh, max.   (run continued)
claude -p --permission-mode bogus→ error: option '--permission-mode <mode>' argument 'bogus' is invalid. Allowed choices are
                                     acceptEdits, auto, bypassPermissions, manual, dontAsk, plan.
claude -p --model not-a-real-model → stderr [claude-code:unrecognized_model] {"model":"not-a-real-model","query_source":"sdk"};
                                     json: is_error true, api_error_status 404, terminal_reason "api_error", exit 0
env -i HOME=… PATH=/usr/bin:/bin:/usr/sbin:/sbin:<caskdir> claude -p … → OK (auth via keychain works in a minimal env)
resume tests                     → secret word recalled across processes and from another cwd; --model sonnet on resume used
                                     claude-sonnet-5-5; --session-id reuse → "Error: Session ID … is already in use." (exit 1);
                                     unknown id → "No conversation found with session ID: …" (exit 1)
concurrent resume                → second process succeeded while the first was running; transcript interleaved (no lock)
signals (text-only job)          → INT: exit 0, result subtype error_during_execution / aborted_streaming; TERM: exit 143,
                                     no stdout; KILL: exit 137; all resumable afterwards; no child processes; no orphans
timings                          → claude -p trivial: wall 3.5–6.8 s, duration_ms 1.8–6.9 s, overhead 1.3–2.9 s;
                                     peak RSS 343 MB; claude agents --json 9.8–14.6 s; claude auth status 18.9–35.9 s;
                                     claude doctor 0.77 s; ps -axo 0.032 s
permissions (-p, prompts none)   → manual: touch denied, Write denied (permission_denials populated); acceptEdits: Write ok;
                                     dontAsk + disallow Edit/Write: git status ok, touch denied, Write absent; --restricted: no Bash;
                                     plan: wrote ~/.claude/plans/<name>.md and could not call ExitPlanMode
stream-json input                → two user turns in one process (same session_id); control_request interrupt → control_response
                                     {"subtype":"success","request_id":"int-1","response":{"still_queued":[]}} and result
                                     terminal_reason "aborted_streaming"; --permission-prompts host emitted system/permission_denied
                                     instead of a control_request
hidden commands/flags            → --advisor fable accepted (exit 0); claude daemon status → "not running"; --name accepted with -p
```

## Appendix B: Observed JSON shapes (CLI 2.1.292)

### B.1 `--output-format json` result (keys)

`type:"result"`, `subtype` (`success` | `error_max_budget_usd` | `error_during_execution` | …), `is_error`, `terminal_reason` (`completed` | `api_error` | `budget_exhausted` | `aborted_streaming`), `api_error_status` (int|null), `stop_reason`, `result` (string|null), `session_id`, `uuid`, `num_turns`, `duration_ms`, `duration_api_ms`, `ttft_ms`, `ttft_stream_ms`, `time_to_request_ms`, `first_content_frame_ms`, `queued_turn_count`, `result_index`, `total_cost_usd`, `usage{input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, output_tokens_details{thinking_tokens}, cache_creation{ephemeral_1h_input_tokens, ephemeral_5m_input_tokens}, server_tool_use{web_search_requests, web_fetch_requests}, service_tier, inference_geo, speed, iterations[], fallback_credit}`, `modelUsage{<model>{inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens, webSearchRequests, costUSD, contextWindow, maxOutputTokens, thinkingTokens, canonicalModel, provider, costBasis}}`, `permission_denials[{tool_name, tool_use_id, tool_input}]`, `fast_mode_state`, `fast_mode_disabled_reason`, `subagent_stats{…}`, `safety_stops`.

### B.2 stream-json `system/init` (keys)

`type, subtype:"init", uuid, session_id, cwd, model, permissionMode, apiKeySource, claude_code_version, tools[], slash_commands[], terminal_slash_commands[], mcp_servers[{name,status}], plugins[{name,path,source,version}], agents[], skills[], memory_paths[], output_style, view_mode, capabilities[] (e.g. interrupt_receipt_v1, interrupt_cancel_queued_v1, interrupt_send_now_v1, msg_lifecycle_v1, sdk_mcp_tools_list_changed, sdk_mcp_manifests, mcp_read_resource_v1, mcp_tool_ui_meta_v1, ui_surface_v1), per_turn_effort_active, fast_mode_state, fast_mode_disabled_reason, analytics_disabled, product_feedback_disabled, messaging_socket_path`.

### B.3 Other stream events

- `system/status {status:"requesting"}`; `system/hook_started|hook_response {hook_name, hook_event, exit_code, outcome}`; `system/permission_denied`; `system/api_retry` (docs).
- `assistant {message:{model, usage{…}, content:[{type:text|tool_use|thinking}], stop_reason}, request_id, parent_tool_use_id}`.
- `user` (tool results).
- `stream_event {event:{type: message_start|content_block_start|content_block_delta{delta:{type:"text_delta",text}}|content_block_stop|message_delta|message_stop}}`.
- `rate_limit_event {rate_limit_info:{status, resetsAt, rateLimitType, overageStatus, overageDisabledReason, isUsingOverage, unifiedWindows:{five_hour:{utilization,resetsAt}, seven_day:{…}, seven_day_overage_included:{…}}}}` — **undocumented**.
- `control_response {response:{subtype, request_id, response:{still_queued:[]}}}` — **undocumented** (stream-json input only).

### B.4 `claude agents --json`

Array of `{pid, cwd, kind ("interactive"|"background"), startedAt (ms epoch), sessionId, name, status ("idle"|"busy"|…)}`; docs add `id`, `state` (`working|blocked|done|failed|stopped`) for background sessions.

## Appendix C: References checked (fetched 2026-10-16)

- Claude Code: `https://code.claude.com/docs/en/cli-reference`, `/headless`, `/model-config`, `/permissions`, `/permission-modes` (linked), `/settings-reference`, `/env-vars`, `/sessions`, `/authentication`, `/agent-view`, `/advisor`, `/costs`, `/agent-sdk/overview`, `/agent-sdk/permissions`, `/agent-sdk/user-input`.
- Telegram: `https://core.telegram.org/bots/api` (Bot API 10.3, 2026-08-24), `/bots/faq`, `/bots/features`, `/bots/api-changelog`; server source `tdlib/telegram-bot-api` (`Client.cpp`, `Client.h`, `Query.cpp`) for 409/429 strings.
- Go: `https://pkg.go.dev/os/exec`, `/log/slog`, `https://go.dev/dl/`, `https://go.dev/doc/devel/release`; module metadata from `https://proxy.golang.org/<module>/@latest` for go-telegram/bot v1.27.0, telegram-bot-api/v5 v5.5.1, telego v1.12.1, telebot.v4 beta.10, modernc.org/sqlite v1.60.1, mattn/go-sqlite3 v1.14.52, gopsutil/v4 v4.26.9, gofrs/flock v0.13.1, x/sys v0.49.0, godotenv v1.5.1, caarlos0/env v11.4.1, google/uuid v1.6.0, testify v1.12.1; `github.com/go-telegram/bot` README/go.mod/sources.
- Local: `man launchd.plist`, `man launchctl`, `man 2 kill`.

## Appendix D: Glossary

- **Logical session (S-id)**: the bot's conversation container; maps 1:1 to a Claude session UUID and a workspace.
- **Claude session**: the CLI transcript identified by a UUID; resumable with `--resume`.
- **Job (j-id)**: one child-process run for one prompt or probe.
- **Tier/mode**: `observe`, `assisted`, `automation` → specific CLI permission flags.
- **Workspace**: a configured, canonicalized root directory; the cwd of every job in a session.
- **Probe**: a `--max-turns 1 --no-session-persistence` call used only to validate a model.

---

## Implementation status (2026-10-16)

Phase 1 is implemented on the local `main` branch. README.md, docs/RUNBOOK.md and docs/TESTING.md describe how to run and verify it. Deviations from this specification:

- **Git remote.** The owner explicitly asked for `origin` to be set to `https://github.com/Publicthrone547/clawdyX.git`, which overrides §1.3 and §17.4. Nothing has been pushed; pushing stays a manual step for the owner.
- **Confirmations** are held in memory, HMAC-bound, one-time, and expire after 120 s. They are not stored in SQLite, so a restart invalidates pending confirmations. That is the safe failure mode.
- **Additional per-job flags.** `--settings` also sets `permissions.blockReadsOutsideWorkingDirectories: true` and `disableAutoMode`. Live tests confirmed that both the Read tool and Bash `cat` are blocked outside the workspace.
- **Secret-path deny rules** include `~/.claude`. As a result, Claude Code's auto-memory writes are denied and reported as denials.
- **Per-job cost** is not stored, because `total_cost_usd` is cumulative per session when resuming. `/usage` instead sums the latest session-level estimates, with a label.
- **`/model reset`** clears a session override. `default` is passed through as the CLI alias.
- **Telegram offset.** The library confirms updates when it next polls, so an update can be lost if the bot crashes mid-handling. Duplicates can never run twice because of update deduplication and the UNIQUE job key.
- **Automation tier (added on owner request).** It is gated by `modes.automation_enabled` and needs a per-session confirmation. It uses `--permission-mode auto --permission-prompts none`, keeps every deny rule and secret path, and drops the read-outside-workspace block. A live test showed that opening Firefox and reading a named outside file were allowed, while `rm` was denied.
- **`/open ADDRESS`** is owner-typed and opens only http(s) URLs via `/usr/bin/open -a <browser_app> -- <url>`. Claude itself still cannot run `open` outside automation.
- **Screen control (added on owner request).** It is gated by `desktop.screen_control` and offered only in automation sessions. The bot binary serves an MCP stdio server (`mcp-screen`) with screenshot, click, move, scroll, type and key tools, built on `screencapture`, `sips` and JXA CGEvent / System Events. Claude Code receives it through `--mcp-config` with `--strict-mcp-config` and a safety system prompt. No allow rule is added, so auto mode's classifier still reviews these calls. A live run confirmed the real CLI reaches the tool. Owner-typed `/screenshot` sends a photo to the private chat.
- **Not built yet (Phase 2+):** attaching external sessions (`/resume <uuid>` currently only switches between bot sessions), `/run` capabilities, stopping background agents with `claude stop`, the approval relay, warm sessions, and attachments.

## Implementation status (v0.4.0, 2026-10-18)

The v2 product plan (desktop app plus worker) is implemented. Deviations from the plan, with reasons:

- **Worker registration uses a LaunchAgent written by the bundled worker** (`clawdyx service install`, label `local.clawdyx`, program `/Applications/clawdyX.app/Contents/Helpers/clawdyx`), not `SMAppService.agent`. The LaunchAgent path was already proven on this Mac, needs no cgo bridge, and works for ad-hoc-signed local builds; the app still shows it in Settings and can start, stop, re-point and uninstall it. `SMAppService.agent` remains an option for signed distribution builds.
- **Screen Recording check:** inferring the permission from window titles (JXA) proved wrong on macOS 27 (it reported "granted" while captures were refused). The worker now reports the state from the last real capture, or from the bundled helper's `CGPreflightScreenCaptureAccess` when the app is installed; otherwise "unknown".
- **App memory is above the plan's 60 MB budget** (about 120–137 MB plus WebKit helpers; see `docs/RELEASE.md`). Destroying the window on close did not reduce it. The worker (29 MB) is independent of the app.
- **Token storage:** the Keychain item is created through `/usr/bin/security` (go-keyring), so the worker and the app can read it without prompts and rebuilds do not invalidate it; any process of the same macOS user can also read it with the same tool.
- **Windows (0.4.0, preview, untested on real hardware):** the worker and the app cross-compile with plain `GOOS=windows` (Wails v3 needs no CGO there). Platform files replace process groups (`taskkill /T /F`), launchd (a per-user Run entry plus a detached worker, stopped through `worker.stop`), sysctl/ps (Win32 calls) and the peer-uid check (the socket file's ACL in `%LOCALAPPDATA%`). Screens and the camera are macOS only. See `docs/RUNBOOK.md#windows`.
- **Camera (0.4.0, opt-in):** `desktop.camera` (Settings → Features → Camera in the app, 0.4.1) enables owner-typed `/cameras` and `/camera [N] [video SECONDS]` through a second bundled helper (`clawdyx-camera`, AVFoundation). It follows the screen rules (one broker, memory only for at most five minutes, audit rows, `/screen off`). Claude gets no camera tool; the helper captures only when the worker is its parent. The automation tier's shell access remains a residual path (`docs/PRIVACY.md#camera`).
- **Not built yet:** command palette, chat companion view, native notifications, periodic monitor refresh, attaching external Claude sessions, warm sessions, approval relay, self-update, screens and camera on Windows.
