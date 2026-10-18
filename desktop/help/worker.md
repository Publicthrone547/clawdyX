# Background worker

The bot runs in the worker, `{{.Paths.Worker}}`, not in this app. You can close or quit the app; Telegram keeps working. The app talks to the worker over a private local socket.

## How it starts

{{if .Darwin -}}
The worker is a LaunchAgent (label `local.clawdyx`). It starts at login, restarts after a crash, and runs only while you are logged in, because Claude Code's login lives in your login Keychain. It does not restart after a clean exit (`/shutdown`).
{{- else if .Windows -}}
The worker starts at login from an entry in your account's startup programs (value `local.clawdyx` under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`), without a console window and without administrator rights. A small supervisor keeps it running: after a crash it starts the worker again after 10 seconds, doubling the wait up to 5 minutes while it keeps crashing. It does not run while you are signed out.
{{- else -}}
The worker is a systemd user service, `clawdyx.service`. It starts when you log in and restarts after a crash. On a server without a desktop login, let it run from boot with `loginctl enable-linger $USER`. Without a systemd user manager, an autostart entry starts it with your desktop session instead.
{{- end}}

## Start, stop, restart

**Settings → Background worker** has Start, Stop, Restart and Uninstall, and shows which program the service runs. In {{.W.Terminal}} (with the `clawdyx` command from [Terminal setup](help:terminal)):

{{if .Windows -}}
```powershell
clawdyx service status
clawdyx service restart
clawdyx service stop      # stays off until service start
```
{{- else -}}
```sh
clawdyx service status
clawdyx service restart
clawdyx service stop      # stays off until service start
```
{{- end}}

Stopping the bot (Overview → Stop) is different: the worker stays running but stops answering Telegram until you press Start. That choice is remembered across restarts.

## Logs

The worker writes `bot.log` (JSON lines, rotated at 20 MB, five files kept) to `{{.Paths.Logs}}`. **Diagnostics** shows a redacted copy. {{if .Linux}}Crash output goes to the journal: `journalctl --user -u clawdyx`.{{else if .Windows}}Crash output goes to `stderr.log` in the same folder, and the supervisor's restarts to `supervisor.log`.{{else}}Crash output goes to `stderr.log` in the same folder.{{end}}

## After an update

Updating the app replaces the worker too. {{if .Windows}}The installer stops the worker before copying and starts it again afterwards.{{else}}Restart the worker afterwards: **Settings → Background worker → Restart**.{{end}}
