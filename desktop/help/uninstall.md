# Uninstall

Claude Code, its history and your projects are never touched.

## With the app

1. **Settings → Background worker → Uninstall…**. Tick "Also delete clawdyX's state (sessions, jobs, audit log) and logs" to remove those too.
{{if .Darwin -}}
2. Quit clawdyX and move it from Applications to the Trash.
{{- else if .Windows -}}
2. Quit clawdyX, then remove it in Windows Settings → Apps → Installed apps → clawdyX → Uninstall. The uninstaller can also delete the settings, the state and the token. (From the `.zip`: delete the `clawdyX` folder.)
{{- else -}}
2. Quit clawdyX and remove the package (for example `sudo apt remove clawdyx`, `sudo dnf remove clawdyx`), or delete the AppImage.
{{- end}}
3. Remove the token and the settings (below), then revoke the token in [@BotFather](open:botfather) with `/revoke`.

## In {{.W.Terminal}}

{{if .Windows -}}
```powershell
clawdyx service uninstall -purge-state     # stop the worker, remove the login entry, delete state and logs
clawdyx token delete                       # remove the token from the Credential Manager
Remove-Item -Recurse "$env:APPDATA\clawdyx"   # the settings
```
{{- else if .Darwin -}}
```sh
clawdyx service uninstall --purge-state    # stop the worker, remove the LaunchAgent, delete state and logs
clawdyx token delete                       # remove the token from the Keychain
rm -r ~/.config/clawdyx                    # the settings
```
{{- else -}}
```sh
clawdyx service uninstall --purge-state    # stop the worker, remove the systemd unit, delete state and logs
clawdyx token delete                       # remove the token from the keyring (or its file)
rm -r ~/.config/clawdyx                    # the settings
```
{{- end}}

Run `clawdyx token delete` before deleting the settings: the token's entry belongs to that settings file.
