# Terminal setup

Everything the app does can also be done in {{.W.Terminal}} with the worker program, `clawdyx`.

## Make the command available

{{if .Darwin -}}
The worker lives inside the app. Add this line to `~/.zshrc`, then open a new Terminal window:

```sh
alias clawdyx="{{.Paths.Worker}}"
```
{{- else if .Windows -}}
The worker lives in the app's `helpers` folder. Add this line to your PowerShell profile (`notepad $PROFILE`), then open a new PowerShell window:

```powershell
Set-Alias clawdyx "{{.Paths.Worker}}"
```
{{- else -}}
The packages install the worker as `{{.Paths.Worker}}`, which is already on your `PATH`. From the `.tar.gz`, copy `clawdyx` to `~/.local/bin`.
{{- end}}

## Set up without the app

{{if .Windows -}}
```powershell
clawdyx setup              # token, your Telegram ID, project folder; then the checks
clawdyx service start      # run in the background now and at every login
clawdyx service status
```
{{- else -}}
```sh
clawdyx setup              # token, your Telegram ID, project folder; then the checks
clawdyx service start      # run in the background now and at every login
clawdyx service status
```
{{- end}}

`clawdyx run` runs the bot in the foreground instead (Ctrl-C stops it). `clawdyx check` runs read-only checks of the settings, the token, Claude Code and Telegram.{{if .Linux}} On a server, `loginctl enable-linger $USER` keeps the worker running while nobody is logged in.{{end}}

## Everyday commands

{{if .Windows -}}
```powershell
clawdyx paths                                   # where the settings, state and logs are
Get-Content -Wait -Tail 20 (Join-Path (clawdyx paths logs) bot.log)   # follow the log
clawdyx ctl status.get                          # the overview as JSON
clawdyx ctl diagnostics.run                     # the checks of Diagnostics
clawdyx backup                                  # a copy of the state database
```
{{- else -}}
```sh
clawdyx paths                                   # where the settings, state and logs are
tail -f "$(clawdyx paths logs)/bot.log"         # follow the log
clawdyx ctl status.get                          # the overview as JSON
clawdyx ctl diagnostics.run                     # the checks of Diagnostics
clawdyx backup                                  # a copy of the state database
```
{{- end}}

`clawdyx ctl help` lists every method of the control socket. Settings are changed in this app or with `clawdyx ctl settings.set`; the settings file is checked strictly, and a mistake there stops the worker from starting.
