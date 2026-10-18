# Security policy

clawdyX lets a Telegram account drive the Claude Code CLI on your computer (macOS, Windows or Linux). Treat a bug that widens who can do that, or what they can do, as a security issue.

## Reporting a vulnerability

Please report privately: use GitHub's **Report a vulnerability** button on the repository's Security tab (private vulnerability reporting). Do not open a public issue for security problems. Include the clawdyX version (`clawdyx version`), the operating system and its version (for Linux also the distribution and whether the session is X11, Wayland or none), the Claude Code version, and the smallest steps that reproduce the problem. Do not include real bot tokens, Telegram IDs, transcripts or screenshots of your desktop.

There is no bug bounty. Reports are handled on a best-effort basis by the maintainer.

## In scope

- The Telegram authorization gate: any update from a sender who is not on the configured allowlist, from a group, or with a forged callback that causes a reply, a lookup, a job, a capture or any other side effect.
- Token handling: the bot token appearing in logs, replies, diagnostics, child-process environments, crash output, settings exports or anywhere other than its configured store (the Keychain, the Credential Manager, the Secret Service keyring, or the 0600 token file).
- Roles: a `user`-role account running an owner-only command, confirming someone else's confirmation, or stopping someone else's job.
- Permission tiers: a way to run commands, edit files, read secret paths (SSH and cloud keys, the token stores, browser profiles, Claude Code's folder, clawdyX's own state) or reach bypass-permissions mode beyond what the selected tier documents. This includes the built-in per-OS deny lists and secret paths (for example Windows' `powershell`, `cmd` and Claude Code's PowerShell tool, Linux's `apt`, `pkexec`, `systemctl`): a command form that slips past them is in scope.
- Path handling: workspace escapes through `..`, symlinks, junctions, case differences or races.
- Screen features and the camera: any capture that happens without an explicit action by the owner, a capture reaching a destination other than the one documented, or the camera helper capturing when the worker did not start it.
- The local control socket: access by another user. On macOS and Linux the worker checks the peer's uid; on Windows the socket's folder ACL (your profile, SYSTEM and administrators) is the only barrier, so anything that weakens it is in scope.
- Process control: clawdyX signalling or ending a process it did not start (it acts only on pids it recorded), and the Windows supervisor's stop file being honoured from somewhere other than the state folder.
- The installers and packages: an installer step that needs administrator rights, writes outside the user's profile (Windows), or deletes data of another installation.

## Out of scope

- Anything an allowed Telegram user can do within the tier the owner selected. The automation tier deliberately lets Claude run shell commands that Claude Code's own safety classifier approves.
- Weaknesses in Claude Code, Telegram, the operating system or third-party modules themselves (report those upstream).
- A compromised user account. clawdyX runs as you and trusts processes running as you; on Windows that includes administrators of the PC.
- Prompt injection that stays within the tier's documented capabilities.

## Hardening notes for operators

- Keep the bot in `observe` unless you need more; enable `automation`, screen features and the camera only when you use them.
- If the token leaks, revoke it in @BotFather (`/token`) and store the new one in the app (Telegram → Bot token) or with `clawdyx ctl telegram.token.set`. A webhook appearing on your bot, or a "duplicate poller" warning in `/status`, means someone else has the token.
- macOS: sign the binary with a stable code-signing identity if you grant it Screen Recording or Accessibility (see `docs/RUNBOOK.md`).
- Windows: run clawdyX as a standard user; it never needs administrator rights.
- Linux servers: a token in the private `.env` file is as safe as your account; keep the home folder private.
