# Security policy

clawdyX lets a Telegram account drive the Claude Code CLI on your Mac. Treat a bug that widens who can do that, or what they can do, as a security issue.

## Reporting a vulnerability

Please report privately: use GitHub's **Report a vulnerability** button on the repository's Security tab (private vulnerability reporting). Do not open a public issue for security problems. Include the version (`clawdyx version`), macOS version, Claude Code version, and the smallest steps that reproduce the problem. Do not include real bot tokens, Telegram IDs, transcripts or screenshots of your desktop.

There is no bug bounty. Reports are handled on a best-effort basis by the maintainer.

## In scope

- The Telegram authorization gate: any update from a sender who is not on the configured allowlist, from a group, or with a forged callback that causes a reply, a lookup, a job, a capture or any other side effect.
- Token handling: the bot token appearing in logs, replies, diagnostics, child-process environments, crash output or files other than its configured store.
- Permission tiers: a way to run commands, edit files, read secret paths (`~/.ssh`, `~/.aws`, keychains, browser profiles, `~/.claude`, clawdyX's own state) or reach bypass-permissions mode beyond what the selected tier documents.
- Path handling: workspace escapes through `..`, symlinks or races.
- Screen features: any capture that happens without an explicit action by the owner, or a capture reaching a destination other than the one documented.
- The local control socket (when present): access by another macOS user.
- Process control: clawdyX signalling a process it did not start.

## Out of scope

- Anything an allowed Telegram user can do within the tier the owner selected. The automation tier deliberately lets Claude run shell commands that Claude Code's own safety classifier approves.
- Weaknesses in Claude Code, Telegram, macOS or third-party modules themselves (report those upstream).
- A compromised macOS user account. clawdyX runs as you and trusts processes running as you.
- Prompt injection that stays within the tier's documented capabilities.

## Hardening notes for operators

- Keep the bot in `observe` unless you need more; enable `automation` and screen control only when you use them.
- If the token leaks, revoke it in @BotFather (`/token`) and run `make setup` to store the new one. A webhook appearing on your bot, or a "duplicate poller" warning in `/status`, means someone else has the token.
- Sign the binary with a stable code-signing identity if you grant it Screen Recording or Accessibility (see `docs/RUNBOOK.md`).
