# Permissions

clawdyX can never grant itself a permission. {{.W.OSName}} asks you, on {{.W.TheComputer}}, or you allow it yourself.

{{if .Darwin -}}
| For | Permission | Where |
| --- | --- | --- |
| Screenshots | Screen & System Audio Recording | System Settings → Privacy & Security → [Screen & System Audio Recording](open:screen-recording) |
| Claude's mouse and keyboard tools | Accessibility | System Settings → Privacy & Security → [Accessibility](open:accessibility) |
| Typing text | Automation (System Events) | macOS asks the first time; System Settings → Privacy & Security → [Automation](open:automation) |
| `/camera` | Camera | System Settings → Privacy & Security → [Camera](open:camera) |

Grant them to **clawdyX**: the worker and its capture helpers live inside the app. After you change one, restart the worker (**Settings → Background worker → Restart**).

- macOS 15 and later ask about once a month whether clawdyX may keep capturing the screen. Until someone answers on the Mac, captures from Telegram fail with a timeout.
- macOS ties the grants to the app's signature. After installing a differently signed build you may have to grant them again.
- Screen Recording can only be checked by capturing, so **Diagnostics** shows the result of the last capture.
{{- else if .Windows -}}
| For | Permission | Where |
| --- | --- | --- |
| Screenshots, mouse and keyboard | none | — |
| `/camera` | Camera access for desktop apps | Windows Settings → Privacy & security → [Camera](open:camera) → "Let desktop apps access your camera" |
| Starting at login | Startup apps | Windows Settings → Apps → [Startup](open:startup-apps): clawdyX must be on |

clawdyX never needs administrator rights. Windows does not let a normal program control windows that run as administrator, so Claude cannot click or type into them (and should not).
{{- else -}}
| For | Permission | Where |
| --- | --- | --- |
| Screenshots on X11, mouse and keyboard | none | — |
| Screenshots on Wayland | the desktop's screenshot permission | the desktop may ask on the screen; answer it there |
| `/camera` | access to the camera device | membership of the `video` group: `sudo usermod -aG video $USER`, then log in again |
| The token | an unlocked keyring | the keyring unlocks when you log in; on a server the token is kept in a private file |

Check the camera access with `ls -l /dev/video*` and `id -nG`: the devices usually belong to the `video` group, and your groups must include it.
{{- end}}

**Diagnostics** in this app shows each permission and, where {{.W.OSName}} has one, a button that opens the right settings page.
