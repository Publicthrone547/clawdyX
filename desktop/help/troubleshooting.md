# Troubleshooting

Start with **Diagnostics**: it runs every check and shows a fix for each problem. In Telegram, `/doctor` shows the same. Each entry below ends with a command that checks it in {{.W.Terminal}}.

## The worker does not start

| Cause | Fix | Check |
| --- | --- | --- |
| The settings file has a mistake | Fix it in **Settings** (it opens without the worker), or reset the section | `clawdyx check` |
| No bot token is stored | **Telegram → Bot token**, or `clawdyx setup` | `clawdyx check` |
| Another copy is running | Stop the other copy (a `clawdyx run` in a terminal) | `clawdyx service status` |
{{- if .Darwin}}
| The LaunchAgent was switched off | **Settings → Background worker → Start** | `launchctl print gui/$(id -u)/local.clawdyx` |
{{- else if .Windows}}
| Git for Windows is missing (Claude Code's shell) | `winget install Git.Git`, or set its `bash.exe` in **Settings → Claude Code** | `Test-Path "$env:ProgramFiles\Git\bin\bash.exe"` |
| The login entry was turned off | Windows Settings → Apps → Startup: turn clawdyX on | `clawdyx service status` |
{{- else}}
| The keyring is locked (the token cannot be read) | Log in to the desktop once, or keep the token in a file: `clawdyx setup -token-file` | `systemctl --user status clawdyx` |
| No systemd user manager | Use the desktop autostart, or `loginctl enable-linger $USER` on a server | `systemctl --user is-system-running` |
{{- end}}

## Telegram

| Symptom | Cause and fix | Check |
| --- | --- | --- |
| "Duplicate poller detected", Telegram error 409 | Another program polls with the same token: a second copy, or a leaked token. Stop it, or replace the token. | `/status` in Telegram |
| No reply at all | The bot is stopped (Overview → Start), or your ID is not on the allowlist | `clawdyx ctl status.get` |
| Replies arrive slowly in bursts | Telegram allows about one message per second per chat | — |

## Claude Code

| Symptom | Cause and fix | Check |
| --- | --- | --- |
| "not logged in" | Sign in again: run `claude` in {{.W.Terminal}} | `claude auth status` |
| Claude Code not found | Install it: `{{.W.InstallClaude}}`, or set its location in **Settings → Claude Code** | `claude --version` |
| "N tool call(s) denied" | Expected in `observe`; use `/mode assisted` for edits | — |

## Screens and camera

{{if .Darwin -}}
| Symptom | Cause and fix | Check |
| --- | --- | --- |
| Screenshots fail or time out | Screen Recording is missing, or the monthly macOS question is waiting on the Mac. Allow clawdyX in System Settings → Privacy & Security → Screen & System Audio Recording, then restart the worker. | `clawdyx ctl diagnostics.run` |
| Clicks and typing do nothing | Accessibility (and, for typing, Automation) is missing for clawdyX | `clawdyx ctl diagnostics.run` |
| `/camera` is refused | The Camera permission was refused: System Settings → Privacy & Security → Camera | `clawdyx ctl diagnostics.run` |
| The camera times out | Another app uses the camera, or macOS is asking on the Mac | — |
{{- else if .Windows -}}
| Symptom | Cause and fix | Check |
| --- | --- | --- |
| Screenshots fail | The PC is locked or on the sign-in screen; unlock it | `clawdyx ctl diagnostics.run` |
| Clicks and typing do nothing in one window | That window runs as administrator; Windows blocks input from normal programs | — |
| `/camera` is refused | Camera access for desktop apps is off: Windows Settings → Privacy & security → Camera | `clawdyx ctl diagnostics.run` |
| Clips fail on a Windows N edition | The H.264 encoder is missing: install the Media Feature Pack (Settings → Apps → Optional features) | — |
| The camera is busy | Another app (a video call) uses it | — |
{{- else -}}
| Symptom | Cause and fix | Check |
| --- | --- | --- |
| "no graphical session" | The worker started without a desktop (a server, or before login). Screen features need a desktop session; restart the worker after logging in. | `echo $XDG_SESSION_TYPE $DISPLAY $WAYLAND_DISPLAY` |
| Screenshots refused on Wayland | The desktop's screenshot dialog was refused or not answered; answer it on the computer | `echo $XDG_CURRENT_DESKTOP` |
| Clicks and typing are not available | Wayland; log in to an X11 session for them | `echo $XDG_SESSION_TYPE` |
| `/camera` is refused | Your user is not in the `video` group: `sudo usermod -aG video $USER`, log in again | `ls -l /dev/video*; id -nG` |
| Clips fail | ffmpeg is not installed | `ffmpeg -version` |
| No {{.W.TrayName}} | GNOME shows tray icons only with the AppIndicator extension; the app works without it (closing the window then quits it) | — |
{{- end}}

## Updates

**Settings → About clawdyX → Check for updates** asks GitHub only when you click it. {{.W.UpdateHint}}{{if .Darwin}} A downloaded build that macOS refuses to open: System Settings → Privacy & Security → **Open Anyway**.{{else if .Windows}} If SmartScreen warns about the download: **More info → Run anyway**.{{end}}
{{if .Windows}}
## Folders

- Keep workspaces outside OneDrive: syncing a folder that Claude edits causes conflicts and slows both down.
- Very long paths (over 260 characters) need long path support turned on in Windows. Check: `Get-ItemPropertyValue HKLM:\SYSTEM\CurrentControlSet\Control\FileSystem LongPathsEnabled` (1 means on).
{{end}}
## Still stuck

**Diagnostics → Create report…** prepares a report with tokens removed. You see it in full before you save it; nothing is uploaded.
