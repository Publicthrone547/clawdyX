# clawdyx runbook

All commands run as the logged-in user. The sections below are per operating system ([macOS](#macos), [Windows](#windows), [Linux desktop](#linux-desktop), [Linux server](#linux-server-headless)); what follows first is the same everywhere.

`clawdyx paths` prints the folders of the configuration you run (`clawdyx paths logs` prints one, for scripts). `CLAWDYX_CONFIG=<file>` (or `-config <file>`) selects another configuration: its token, state and logs are its own, so a test instance never touches the real one.

## Setup without the app

```sh
clawdyx setup          # token, your Telegram ID, project folder; then the read-only checks
clawdyx service start  # the background worker: now and at every login
```

`clawdyx setup` asks for the bot token (hidden input) and your numeric Telegram user ID. If you leave the ID empty, send any message to the bot and press Enter: the wizard reads the sender ID from Telegram and acknowledges the pending updates so they are not replayed as prompts later. It then verifies the token (`getMe`), writes the private files and runs `clawdyx check`.

Scripted: `printf '%s\n' "$TOKEN" | clawdyx setup -token-stdin -owner-id 123456789 -workspace ~/projects/app [-browser NAME] [-claude-binary PATH] [-no-check] [-force]` (`-headless` on a server, `-token-file` to keep the token in a private file). Everything else is changed afterwards in the app's Settings or with `clawdyx ctl settings.set`; the settings file is validated strictly, and a mistake there stops the worker from starting.

Create the bot in Telegram with @BotFather (`/newbot`) and copy the token. Never paste it into chats, issues, files or the repository. The bot refuses to start without an owner, and only the listed workspaces can ever be used.

`clawdyx check` validates the settings and the token format, runs `claude --version` and `claude auth status` (about 30 s), calls Telegram `getMe`/`getWebhookInfo`, and checks the instance lock. It changes nothing.

## Control socket (`clawdyx ctl`)

The running worker listens on `<state>/control.sock` (mode 0600 in a 0700 folder; on macOS and Linux connections from other users are refused by uid, on Windows the folder's permissions decide). The desktop app uses it; you can too:

```sh
clawdyx ctl status.get                      # one-screen overview as JSON
clawdyx ctl bot.stop                        # stop Telegram polling; running jobs finish
clawdyx ctl bot.stop '{"cancelJobs":true}'  # stop polling and cancel running jobs
clawdyx ctl bot.start                       # validate the token, start polling again
clawdyx ctl diagnostics.run                 # the same checks as the app's Diagnostics page
clawdyx ctl settings.get                    # every setting
clawdyx ctl events                          # stream job/bot/session events until Ctrl-C
clawdyx ctl help                            # every method
```

`bot.stop` is remembered: after a restart or a login the worker stays connected but does not poll Telegram until `bot.start` (or Start in the app). `clawdyx service status` shows the polling state.

## Inspect

- Telegram: `/status`, `/doctor`, `/processes`, `/usage`, `/jobs`, `/logs j-ID`.
- Logs: `bot.log` in the log folder (JSON lines, rotated at 20 MB × 5).

## Back up and restore

```sh
clawdyx backup        # VACUUM INTO <state>/backups/state-<time>.db (keeps 7)
```

To restore: stop the worker (`clawdyx service stop`), copy a backup over `state.db` (delete `state.db-wal` and `state.db-shm`), then `clawdyx service start`. Claude transcripts belong to Claude Code and are not copied.

## Upgrade from 0.4.x

Settings and the state database of 0.4.x are read as they are; there is no database migration. The first settings change in 1.0 rewrites the settings file with every key. Going back to 0.4.1 afterwards works unless the Git Bash location (`claude.git_bash_path`, Windows) was set: 0.4.1 rejects that key, so remove it first. On Windows the settings move to `%APPDATA%\clawdyx` the first time the 1.0 worker is started; see [Windows](#windows).

## Rotate or revoke the Telegram token

1. @BotFather → `/revoke` (or `/token`) → new token.
2. In the app: **Telegram → Bot token → Replace token…**, or `clawdyx ctl telegram.token.set '{"token":"…"}'` (checks it with Telegram first, stores it and restarts the worker), or `clawdyx setup` and choose to replace the stored token.
3. `/doctor` must show `Webhook: none`. At startup the bot deletes any webhook it finds and warns you, since a webhook set by someone else means the old token leaked.
4. Look at the unauthorized-update counter in `/status`. Treat the bot as compromised for the whole exposure window. The owner-only gate still prevented other users from running anything.

## Troubleshoot Claude authentication

`/doctor` shows `loggedIn=false`, or jobs fail with an authentication error: run `claude auth status`, and if needed `claude auth login`. The bot never logs in, never reads the token store for Claude, and never falls back to an API key. If `ANTHROPIC_API_KEY` or `CLAUDE_CODE_OAUTH_TOKEN` is set in the bot's environment, it logs a warning and does not pass the value to `claude`.

## Common problems

| Symptom | Cause / fix |
| --- | --- |
| `another clawdyx instance is running` (exit 3) | A second copy is running: `clawdyx service status`, or stop the foreground copy. |
| `/status` shows "Duplicate poller detected" | Another process polls with the same token: a second instance, or a leaked token. Rotate the token if it is not yours. |
| Jobs show "N tool call(s) denied" | Expected in `observe`. Use `/mode assisted` if edits are wanted, or add a specific rule to the workspace's allowed commands. |
| "Claude session transcript no longer exists" | Claude Code deleted the transcript (for example via `cleanupPeriodDays`). Start fresh with `/new`. |
| Replies arrive slowly in bursts | The bot paces itself to about 1 message/s per chat, as Telegram requires. |

The app's **Help → Troubleshooting** has the per-OS list with a check for each entry.

## macOS

| What | Where |
| --- | --- |
| Token | login Keychain, item `clawdyx` (or `~/.config/clawdyx/.env`, 0600, with `setup -token-file`) |
| Settings | `~/.config/clawdyx/config.json` (0600) |
| State | `~/Library/Application Support/clawdyx/` (`state.db`, `bot.lock`, `control.sock`, `backups/`) |
| Logs | `~/Library/Logs/clawdyx/` (`bot.log`; `stdout.log`, `stderr.log` from launchd) |
| LaunchAgent | `~/Library/LaunchAgents/local.clawdyx.plist`, label `local.clawdyx` |

### Desktop app

```sh
make app && cp -R bin/clawdyX.app /Applications/ && open /Applications/clawdyX.app
make dmg                                    # the disk image with an Applications link
```

- **First launch:** the wizard writes the settings, stores the token in the Keychain, and installs the LaunchAgent for the worker inside the app (`/Applications/clawdyX.app/Contents/Helpers/clawdyx`).
- **Switching from the standalone worker:** Overview → **Use the app's worker** (or `/Applications/clawdyX.app/Contents/Helpers/clawdyx service install`). It re-points the same LaunchAgent at the app's worker. To go back: `~/go/bin/clawdyx service install`.
- **Updating the app:** quit it, replace `/Applications/clawdyX.app`, open it, then Settings → Background worker → Restart (the worker inside the app changed).
- **Permissions:** grant Screen Recording and Accessibility to **clawdyX** (the worker and its capture helpers live inside the app). With the camera on, macOS asks for **Camera** at the first `/camera`; if it was refused, enable clawdyX in System Settings → Privacy & Security → Camera and restart the worker. Diagnostics shows the permission state and opens that page. The standalone `~/go/bin/clawdyx` cannot use the camera (the helper lives in the app).
- **Signing for distribution:** `CLAWDYX_SIGN_IDENTITY="Developer ID Application: … (TEAMID)" CLAWDYX_HARDENED=1 make dmg`, then `xcrun notarytool submit bin/clawdyX-<version>-darwin-arm64.dmg --keychain-profile <profile> --wait` and `xcrun stapler staple …`. Unsigned or ad-hoc builds downloaded from the internet must be allowed once in System Settings → Privacy & Security → **Open Anyway**.

### Background service (LaunchAgent)

```sh
make start      # installs the binary to $(go env GOPATH)/bin, writes the plist, loads it
make stop       # bootout + disable: stays off until make start
make restart    # rebuild, reinstall, restart
make status
make logs
```

`clawdyx service start|stop|restart|status` does the same for an installed worker. `make start` refuses to run while a foreground `clawdyx run` holds the state lock. The plist is regenerated on every start from the binary path and the settings path. Advanced: `launchctl print gui/$(id -u)/local.clawdyx`.

The agent runs only in the logged-in GUI (Aqua) session, because the claude.ai login lives in your login Keychain. It starts at login and restarts after a crash, but not after a clean exit (`/shutdown`): `launchctl kickstart gui/$(id -u)/local.clawdyx`.

### Permissions for screen control (standalone worker)

Under launchd, macOS checks the `clawdyx` program itself, not your terminal. In System Settings → Privacy & Security, add `~/go/bin/clawdyx` to **Accessibility** and **Screen & System Audio Recording** (click **+**, press Cmd-Shift-G and paste the path), then `launchctl kickstart -k gui/$(id -u)/local.clawdyx`. Do not grant your terminal app, because every program started there would get the same control.

macOS ties these grants to the binary's code signature, so an unsigned build loses them on every rebuild. Sign it with a stable identity:

```sh
make identities                                  # lists usable identities
export CLAWDYX_SIGN_IDENTITY="Apple Development: Your Name (TEAMID)"
make restart                                     # installs, signs, restarts
```

### Upgrade, rollback, uninstall

- **Upgrade:** replace the app (above), or `make restart` from source. Claude Code: `brew upgrade --cask claude-code`; within an hour, or at restart, the bot sees the new version, clears its model-validation cache, runs one smoke probe and reports the result.
- **Rollback:** install the previous `.dmg` and restart the worker; the settings and the state database stay readable (see [Upgrade from 0.4.x](#upgrade-from-04x)).
- **Uninstall:** Settings → Background worker → Uninstall (optionally delete state and logs), quit the app, move it to the Trash, then `clawdyx token delete` (or `security delete-generic-password -s clawdyx`) and `rm -r ~/.config/clawdyx`. In Terminal: `clawdyx service uninstall --purge-state`.

## Windows

| What | Where |
| --- | --- |
| Token | Credential Manager, generic credential `clawdyx:telegram-bot-token@…` (or `%APPDATA%\clawdyx\.env` with `setup -token-file`) |
| Settings | `%APPDATA%\clawdyx\config.json` |
| State | `%LOCALAPPDATA%\clawdyx\` (`state.db`, `bot.lock`, `worker.pid`, `supervisor.pid`, `control.sock`, `backups\`) |
| Logs | `%LOCALAPPDATA%\clawdyx\logs\` (`bot.log`; `supervisor.log`; `stderr.log` for crashes) |
| Login entry | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value `local.clawdyx` |
| Installed app | `%LOCALAPPDATA%\Programs\clawdyX\` (`clawdyX.exe`; `helpers\clawdyx.exe`, `clawdyxw.exe`, `clawdyx-camera.exe`) |

- **Background worker.** `clawdyx service start` writes the login entry and starts the supervisor. At login, Windows starts `helpers\clawdyxw.exe` from the entry, a launcher without a console window, which runs `clawdyx service supervise`; the supervisor runs `clawdyx run` and starts it again after a crash: after 10 seconds, doubling up to 5 minutes while it keeps crashing, back to 10 seconds after 10 minutes of uptime. `clawdyx service stop` writes `supervisor.stop`, asks the worker to exit cleanly over the control socket, waits, ends only the processes clawdyX started (from its pid files), and removes the login entry; it stays off until `service start`. No administrator rights are needed for any of it. It does not run while you are signed out.
- **Control socket.** An AF_UNIX socket (Windows 10 1803+). Windows has no peer credentials, so access is decided by the file's ACL: `%LOCALAPPDATA%` is private to your account (plus SYSTEM and administrators). The 0600 file-mode checks are skipped on Windows for the same reason.
- **Claude Code** is looked up on `PATH`, then `%USERPROFILE%\.local\bin\claude.exe`, then `%APPDATA%\npm\claude.cmd`. The native `claude.exe` is recommended: restart recovery only recognises a process named `claude`. Claude Code's shell tool needs Git for Windows; clawdyX finds its `bash.exe` or uses the location set in **Settings → Claude Code** (it passes it as `CLAUDE_CODE_GIT_BASH_PATH`). Children get your `PATH` and the Windows profile variables, never the bot token or API keys.
- **Deny list.** On Windows the built-in deny list adds `powershell`, `pwsh`, `cmd`, `winget`, `reg`, `schtasks` and download tools, and Claude Code's PowerShell tool is disallowed in every tier (Claude Code 2.1.302 has one; `--disallowedTools PowerShell` removes it, verified).
- **Cancelling a job** ends Claude immediately (`taskkill /T /F` on its pid); there is no SIGINT step, so the interrupted turn may be missing from the transcript.
- **Screens and camera.** Screenshots (GDI, every monitor, per-monitor DPI) and the mouse and keyboard tools (SendInput) need no permission; input cannot reach windows that run as administrator, the lock screen or the sign-in screen. The camera helper (Media Foundation) needs camera access for desktop apps (Windows Settings → Privacy & security → Camera); clips are H.264 MP4, which Windows N editions lack without the Media Feature Pack.
- **Standard user, non-ASCII user names, long paths.** Everything runs as a standard user. Paths with spaces and non-ASCII characters (`C:\Users\Иван Петров\…`) work; the login entry quotes every path. Paths longer than 260 characters need long path support turned on in Windows (`LongPathsEnabled`); keep workspaces short and outside OneDrive (syncing a folder Claude edits causes conflicts).

### Settings move from 0.4.x

0.4.x kept the settings in `%USERPROFILE%\.config\clawdyx`. `clawdyx service start` (also `install` and `restart`, and the app's Start) of 1.0 stops the old worker, moves `config.json` and `.env` to `%APPDATA%\clawdyx`, moves the token to the new settings file's Credential Manager entry, and starts the worker on the new path. Until then a 1.0 `clawdyx` reads the old file where it is. `CLAWDYX_CONFIG` disables the move.

### Upgrade, rollback, uninstall

- **Upgrade:** run the new installer. It stops the worker when the login entry runs this install's worker, replaces the files and starts it again. From the `.zip`: `clawdyx service stop`, replace the folder, `clawdyx service start`.
- **Rollback:** install the previous version the same way.
- **Uninstall:** Windows Settings → Apps → Installed apps → clawdyX → Uninstall. It removes the worker's login entry and the app's own, and offers to delete the settings, the state and the token (only when no worker from another folder still uses them). By hand: `clawdyx service uninstall -purge-state`, `clawdyx token delete`, delete `%APPDATA%\clawdyx` and the app's folder.

## Linux desktop

| What | Where |
| --- | --- |
| Token | the Secret Service keyring (GNOME Keyring, KWallet), item `clawdyx`; without one `~/.config/clawdyx/.env` (0600) |
| Settings | `$XDG_CONFIG_HOME/clawdyx/config.json` (`~/.config/clawdyx`) |
| State | `$XDG_STATE_HOME/clawdyx/` (`~/.local/state/clawdyx`: `state.db`, `bot.lock`, `control.sock`, `backups/`) |
| Logs | `~/.local/state/clawdyx/logs/` (`bot.log`) and the journal (`journalctl --user -u clawdyx`) |
| Service | systemd user unit `~/.config/systemd/user/clawdyx.service` (`Restart=on-failure`); without a user manager an autostart entry `~/.config/autostart/clawdyx.desktop` |
| Installed app | `/usr/bin/clawdyX` and `/usr/bin/clawdyx` (packages), `/usr/share/applications/clawdyX.desktop` |

- **Background worker.** `clawdyx service start` writes the unit and runs `systemctl --user enable --now clawdyx.service`. It starts at login and restarts after a crash. `clawdyx service status` shows the unit, the polling state and whether lingering is on. Without a systemd user manager, an XDG autostart entry starts the supervisor with the desktop session instead.
- **Token.** In the keyring when the Secret Service answers on the session bus; otherwise (a server, a minimal desktop) in a private file, and setup says so. A locked keyring after an automatic login keeps the worker from reading the token: log in to the desktop once, or keep the token in a file (`clawdyx setup -token-file`).
- **Screens.** On X11 screenshots of every monitor (RandR) and the mouse and keyboard tools (XTEST) work without permissions. On Wayland screenshots go through the screenshot portal (`org.freedesktop.portal.Screenshot`; the desktop may ask on the screen) or `grim` on wlroots compositors, the whole screen is one display, and the mouse and keyboard tools are not available yet. `list_displays` and Diagnostics name the session.
- **Camera.** Photos come straight from `/dev/video*` (V4L2, pure Go); your user needs the `video` group (`sudo usermod -aG video $USER`, then log in again). Clips are recorded by `ffmpeg` when it is installed.
- **Tray.** The tray icon needs a StatusNotifier host: KDE has one; on GNOME install the AppIndicator extension. Without it closing the window quits the app; the worker keeps running.

### Upgrade, rollback, uninstall

- **Upgrade:** install the new package (`sudo apt install ./clawdyx_<version>_amd64.deb`, `sudo dnf upgrade ./clawdyx-<version>-1.x86_64.rpm`), then `clawdyx service restart`.
- **Rollback:** install the previous package (`sudo apt install --allow-downgrades ./clawdyx_<old>_amd64.deb`), then `clawdyx service restart`.
- **Uninstall:** `clawdyx service uninstall --purge-state`, `clawdyx token delete`, `rm -r ~/.config/clawdyx`, then `sudo apt remove clawdyx` (or `sudo dnf remove clawdyx`).

## Linux server (headless)

The worker alone, without a desktop, as a systemd user service:

```bash
tar -xzf clawdyx-<version>-linux-amd64.tar.gz && install -m 755 clawdyx-<version>-linux-amd64/clawdyx ~/.local/bin/
printf '%s\n' "$TOKEN" | clawdyx setup -headless -token-stdin -owner-id 123456789 -workspace ~/projects/app
clawdyx service start
loginctl enable-linger "$USER"     # once: the user manager runs from boot, without a login
```

- `-headless` (automatic without a graphical session) asks for no browser, turns `/open` off and prints the steps above. Without a keyring the token goes to `~/.config/clawdyx/.env` (0600).
- Over SSH, `systemctl --user` needs the user's session bus: log in as that user directly (not with `su`), or use `machinectl shell user@`.
- Logs: `journalctl --user -u clawdyx -f` and `~/.local/state/clawdyx/logs/bot.log`. `/doctor` reports the session as none (a server): screen features are off; a camera attached to the server works.
- Upgrade: replace `~/.local/bin/clawdyx`, then `clawdyx service restart`. Uninstall: `clawdyx service uninstall --purge-state`, `clawdyx token delete`, `rm -r ~/.config/clawdyx ~/.local/bin/clawdyx`, and `loginctl disable-linger "$USER"` if nothing else needs it.
