# clawdyx runbook

All commands run as the logged-in macOS user (Windows: see [Windows](#windows)). Paths:

| What | Where |
| --- | --- |
| Token | macOS login Keychain, item `clawdyx` (or `~/.config/clawdyx/.env`, 0600, with `setup -token-file`) |
| Config | `~/.config/clawdyx/config.json` (0600) or `$CLAWDYX_CONFIG` |
| State | `~/Library/Application Support/clawdyx/` (`state.db`, `bot.lock`, `backups/`) |
| Logs | `~/Library/Logs/clawdyx/` (`bot.log` JSON, rotated 20 MB × 5; `stdout.log`, `stderr.log` from launchd) |
| LaunchAgent | `~/Library/LaunchAgents/local.clawdyx.plist`, label `local.clawdyx` |

## Desktop app

```sh
make app && cp -R bin/clawdyX.app /Applications/ && open /Applications/clawdyX.app
make dmg                                    # bin/clawdyX-<version>.dmg with an Applications link
```

- **First launch:** the wizard writes `~/.config/clawdyx/config.json`, stores the token in the Keychain, and installs the LaunchAgent for the worker inside the app (`/Applications/clawdyX.app/Contents/Helpers/clawdyx`).
- **Switching from the standalone worker:** Overview → **Use the app's worker** (or `/Applications/clawdyX.app/Contents/Helpers/clawdyx service install`). It re-points the same LaunchAgent (`local.clawdyx`) at the app's worker. To go back: `~/go/bin/clawdyx service install`. (`make start` restarts whatever the service already runs; it writes a new LaunchAgent for `~/go/bin/clawdyx` only when none is loaded.)
- **Updating the app:** quit it, replace `/Applications/clawdyX.app`, open it, then Settings → Background worker → Restart (the worker binary inside the app changed).
- **Permissions:** grant Screen Recording and Accessibility to **clawdyX** (the worker and its capture helpers live inside the app). With `"desktop": {"camera": true}`, macOS asks for **Camera** at the first `/camera`; if it was refused, enable clawdyX in System Settings → Privacy & Security → Camera and restart the worker. The camera needs the app's `clawdyx-camera` helper; the standalone `~/go/bin/clawdyx` replies that it cannot use the camera. Settings → Background worker shows which program the service runs.
- **Signing for distribution:** `CLAWDYX_SIGN_IDENTITY="Developer ID Application: … (TEAMID)" CLAWDYX_HARDENED=1 make dmg`, then `xcrun notarytool submit bin/clawdyX-<version>.dmg --keychain-profile <profile> --wait` and `xcrun stapler staple bin/clawdyX-<version>.dmg`. Unsigned or ad-hoc builds downloaded from the internet must be allowed once in System Settings → Privacy & Security → **Open Anyway** (macOS 15 and later no longer offer Control-click → Open).
- **Uninstall:** Settings → Background worker → Uninstall (optionally delete state and logs), quit the app, move it to the Trash. Remaining: `~/.config/clawdyx/` and the Keychain item `clawdyx` (`security delete-generic-password -s clawdyx`). Claude Code and your projects are never touched.

## Install and first run (CLI)

```sh
make setup      # token, your Telegram ID, project folder → ~/.config/clawdyx/{.env,config.json} (0600); then the read-only checks
make start      # background service (LaunchAgent): starts now and at every login
```

`make setup` asks for the bot token (hidden input) and your numeric Telegram user ID. If you leave the ID empty, send any message to the bot and press Enter: the wizard reads the sender ID from Telegram and acknowledges the pending updates so they are not replayed as prompts later. It then verifies the token (`getMe`), writes the private files and runs `clawdyx check`.

Scripted setup: `printf '%s\n' "$TOKEN" | bin/clawdyx setup -token-stdin -owner-id 123456789 -workspace ~/projects/app [-browser Safari] [-no-check] [-force]`.

### Manual setup (without the wizard)

1. Create the bot in Telegram with @BotFather (`/newbot`) and copy the token. Never paste it into chats, issues, config.json or the repository.
2. Store it:
   ```sh
   mkdir -p ~/.config/clawdyx && chmod 700 ~/.config/clawdyx
   printf 'TELEGRAM_BOT_TOKEN=%s\n' '<token>' > ~/.config/clawdyx/.env && chmod 600 ~/.config/clawdyx/.env
   cp config.example.json ~/.config/clawdyx/config.json && chmod 600 ~/.config/clawdyx/config.json
   ```
3. Edit config.json: set `telegram.owner_user_id` to your numeric Telegram user ID (the bot refuses to start while it is 0) and list your `workspaces`. Only directories listed there can ever be used. Avoid `~/Desktop`, `~/Documents` and `~/Downloads`: macOS privacy protection can block a launchd-started process there.
4. `make doctor` validates the config and token format, runs `claude --version` and `claude auth status` (about 30 s), calls Telegram `getMe`/`getWebhookInfo`, and checks the instance lock. It changes nothing.
5. `make run` runs the bot in the foreground; send `/start` from the owner account.

## Background service (LaunchAgent)

```sh
make start      # installs the binary to $(go env GOPATH)/bin, writes the plist, loads it; restarts an already running service
make stop       # bootout + disable: stays off until make start
make restart    # rebuild, reinstall, restart
make status     # running under launchd / running in the foreground / stopped
make logs
```

`make start` refuses to run while a foreground `clawdyx run` holds the state lock; stop that one with Ctrl-C first. The plist is regenerated on every start from the installed binary path and the config path, so manual edits to it do not survive. Advanced: `launchctl print gui/$(id -u)/local.clawdyx`.

The agent runs only in the logged-in GUI (Aqua) session. That is required, because the claude.ai login lives in your login keychain. It starts at login and restarts after a crash, but not after a clean exit (`/shutdown`). It does not run while you are logged out. If the bot is offline, a Telegram command cannot wake it. Use `launchctl kickstart` from the Mac or log in again.

### macOS permissions for screen control (service)

Under launchd, macOS checks the `clawdyx` program itself, not your terminal. In System Settings → Privacy & Security, add `~/go/bin/clawdyx` to both **Accessibility** (mouse and keyboard) and **Screen & System Audio Recording** (screenshots). Click **+**, press Cmd-Shift-G and paste the path. Then run `launchctl kickstart -k gui/$(id -u)/local.clawdyx`. Do not grant iTerm, because that would give every program started in iTerm the same control.

macOS ties these grants to the exact binary. After `make install` installs a new build, remove `clawdyx` from both lists, add it again, and restart the service.

## Control socket (`clawdyx ctl`)

The running worker listens on `~/Library/Application Support/clawdyx/control.sock` (mode 0600 in a 0700 folder; connections from other macOS users are refused by uid). The desktop app uses it; you can too:

```sh
clawdyx ctl status.get                      # one-screen overview as JSON
clawdyx ctl bot.stop                        # stop Telegram polling; running jobs finish
clawdyx ctl bot.stop '{"cancelJobs":true}'  # stop polling and cancel running jobs
clawdyx ctl bot.start                       # validate the token, start polling again
clawdyx ctl diagnostics.run                 # the same checks as the app's Diagnostics page
clawdyx ctl events                          # stream job/bot/session events until Ctrl-C
clawdyx ctl help                            # every method
```

`bot.stop` is remembered: after a restart or a login the worker stays connected but does not poll Telegram until `bot.start` (or Start in the app). `make status` shows the polling state.

## Inspect

- Telegram: `/status`, `/doctor`, `/processes`, `/usage`, `/jobs`, `/logs j-ID`.
- Logs: `tail -f ~/Library/Logs/clawdyx/bot.log | jq .`
- launchd: `launchctl print gui/$(id -u)/local.clawdyx | head -40`

## Update

- **Claude Code**: `brew upgrade --cask claude-code` in Terminal. Within an hour, or at restart, the bot sees the new version, clears its model-validation cache, runs one smoke probe and reports the result. If the probe fails with an authentication error, check whether `claude -p` changed its default to bare mode. The docs announce that bare mode will become the default for `-p`, and it ignores the claude.ai login. If that happened, pin the previous cask version until the bot passes the documented opt-out.
- **Bot**: `make restart`.

## Back up and restore

```sh
clawdyx backup        # VACUUM INTO ~/Library/Application Support/clawdyx/backups/state-<time>.db (keeps 7)
```

To restore: `launchctl bootout gui/$(id -u)/local.clawdyx`, copy a backup over `state.db` (delete `state.db-wal` and `state.db-shm`), then `clawdyx service install` or `launchctl bootstrap …`. Claude transcripts belong to Claude Code (`~/.claude/projects`) and are not copied.

## Keep macOS permissions across rebuilds

macOS ties Screen Recording and Accessibility grants to the binary's code signature. An unsigned (ad-hoc) Go build gets a new signature on every build, so the grants disappear after each `make install`. Sign it with a stable identity:

```sh
make identities                                  # lists usable identities, e.g. "Apple Development: …"
export CLAWDYX_SIGN_IDENTITY="Apple Development: Your Name (TEAMID)"
make restart                                     # installs, signs, restarts
```

Grant the permissions once more after the first signed build; later signed builds keep them. A free Apple Development certificate (Xcode → Settings → Accounts) or a self-signed code-signing certificate (Keychain Access → Certificate Assistant) both work. The Keychain token is not affected by signing.

## Rotate or revoke the Telegram token

1. @BotFather → `/revoke` (or `/token`) → new token.
2. Run `make setup` and choose to replace the stored token, or `clawdyx ctl telegram.token.set '{"token":"…"}'` (checks it with Telegram first, stores it in the Keychain and restarts the worker).
3. `make restart`.
4. `/doctor` must show `Webhook: none`. At startup the bot deletes any webhook it finds and warns you, since a webhook set by someone else means the old token leaked.
5. Look at the unauthorized-update counter in `/status`. Treat the bot as compromised for the whole exposure window. The owner-only gate still prevented other users from running anything.

## Troubleshoot Claude authentication

`/doctor` shows `loggedIn=false`, or jobs fail with an authentication error. In Terminal run `claude auth status`, and if needed `claude auth login`. The bot never logs in, never reads the keychain, and never falls back to an API key. If `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` is set in the bot's environment, it logs a warning and does not pass the value to `claude`.

## Common problems

| Symptom | Cause / fix |
| --- | --- |
| `another clawdyx instance is running` (exit 3) | A second copy is running. `launchctl print` the agent, or stop the foreground copy. |
| `/status` shows "Duplicate poller detected" | Another process polls with the same token: a second instance, or a leaked token. Rotate the token if it is not yours. |
| Jobs show "N tool call(s) denied" | Expected in `observe`. Use `/mode assisted` if edits are wanted, or add a specific rule to `allowed_commands`. |
| "Claude session transcript no longer exists" | Claude Code deleted the transcript (for example via `cleanupPeriodDays`). Start fresh with `/new`. |
| Replies arrive slowly in bursts | The bot paces itself to about 1 message/s per chat, as Telegram requires. |

## Uninstall / revert

```sh
clawdyx service uninstall                 # bootout + remove plist; state kept
clawdyx service uninstall --purge-state   # also delete state dir and logs
rm -f ~/.config/clawdyx/.env              # token file, if you used -token-file
security delete-generic-password -s clawdyx   # Keychain token (repeat until "could not be found")
# then revoke the token in @BotFather
```

Claude Code itself is never modified.

## Windows

Preview build for Windows 10/11 x64, cross-compiled from macOS with `make build-windows` (plain `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build`; Wails v3 needs no CGO on Windows). **Not yet tested on a real Windows machine.**

| What | Where |
| --- | --- |
| Token | Windows Credential Manager, generic credential `clawdyx` (or `.env` with `setup -token-file`) |
| Config | `%USERPROFILE%\.config\clawdyx\config.json` |
| State | `%LOCALAPPDATA%\clawdyx\` (`state.db`, `bot.lock`, `worker.pid`, `control.sock`) |
| Logs | `%LOCALAPPDATA%\clawdyx\logs\` (`bot.log`; `stderr.log` for crashes of the background worker) |
| Login entry | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `local.clawdyx` |

- **Background worker.** `clawdyx service start` writes the login entry and starts `clawdyx run` as a detached process without a window. At login, Windows starts it from the Run entry; it closes its console window at once (it may flash briefly). `service stop` asks the worker to exit cleanly (`worker.stop` over the control socket), waits up to 40 s, then ends its process tree, and removes the login entry. `service restart` and the app's **Restart** start a fresh worker. There is no automatic restart after a crash: run `clawdyx service start` again (`stderr.log` shows the crash).
- **Control socket.** An AF_UNIX socket (Windows 10 1803+). Windows has no peer credentials, so access is decided by the file's ACL: `%LOCALAPPDATA%` is private to your account (plus SYSTEM and administrators). The 0600 file-mode checks are skipped on Windows for the same reason.
- **Cancelling a job** ends Claude immediately (`taskkill /T /F`); there is no SIGINT step, so the interrupted turn may be missing from the transcript.
- **Claude Code** is looked up on `PATH`, then `%USERPROFILE%\.local\bin\claude.exe`, then `%APPDATA%\npm\claude.cmd`. The native `claude.exe` is recommended: restart recovery only recognises a process named `claude`. Children get your `PATH` and the Windows profile variables (`USERPROFILE`, `APPDATA`, `SystemRoot`, `CLAUDE_CODE_GIT_BASH_PATH`, …), never the bot token or API keys.
- **Not available:** screens and screen control (`/screens`, `/screen`, `/look`, the MCP screen tools) and the camera (`/cameras`, `/camera`) reply that they are macOS only. `/open` always uses the default browser. `/processes` shows memory but no load average.
- **Desktop app.** `clawdyX.exe` looks for the worker in `helpers\clawdyx.exe` next to it (a separate folder because Windows file names ignore case). Some screens still use macOS wording (Keychain, System Settings).
- **Uninstall:** `helpers\clawdyx.exe service uninstall -purge-state`, delete the folder, `%USERPROFILE%\.config\clawdyx`, and the `clawdyx` entry in Credential Manager.

