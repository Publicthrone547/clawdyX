# Getting started

clawdyX lets you use the Claude Code on {{.W.ThisComputer}} from Telegram. The bot runs in a small background worker on {{.W.ThisComputer}}; this app is its control panel. Closing the app never stops the bot.

## What you need

{{if .Darwin -}}
- macOS 14 or later on Apple silicon.
- Claude Code installed and signed in: `claude` works in Terminal. Install it with `{{.W.InstallClaude}}`, then run `claude` once to sign in.
{{- else if .Windows -}}
- Windows 10 (version 1803 or later) or Windows 11, 64-bit.
- Claude Code for Windows installed and signed in: `claude` works in PowerShell. Install it with `{{.W.InstallClaude}}`, then run `claude` once to sign in.
- Git for Windows: Claude Code uses its bash for shell commands (`winget install Git.Git`).
{{- else -}}
- A 64-bit distribution with systemd. The app needs a desktop with GTK 3 and WebKitGTK 4.1; on a server without a desktop, use the worker alone (see [Terminal setup](help:terminal)).
- Claude Code installed and signed in: `claude` works in a terminal. Install it with `{{.W.InstallClaude}}`, then run `claude` once to sign in.
{{- end}}
- A Telegram bot of your own: create it with [@BotFather](open:botfather) (`/newbot`) and keep its token ready.

## Install

{{if .Darwin -}}
1. Download `clawdyX-<version>.dmg` from the [release page](open:releases), open it and drag clawdyX to Applications.
2. Open clawdyX. Release builds are not notarized yet: if macOS refuses to open it, open System Settings → Privacy & Security and click **Open Anyway**.
{{- else if .Windows -}}
1. Download `clawdyX-<version>-windows-amd64-setup.exe` from the [release page](open:releases) and run it. It installs for your account only, without an administrator prompt. (The `.zip` works too: extract the `clawdyX` folder somewhere permanent and open `clawdyX.exe`.)
2. The build is not code-signed yet, so SmartScreen may warn: click **More info → Run anyway**.
{{- else -}}
1. Download the package for your distribution from the [release page](open:releases): `.deb` (Debian, Ubuntu), `.rpm` (Fedora, openSUSE), the AppImage, or the `.tar.gz`.
2. Install it with your package manager (for example `sudo apt install ./clawdyx_<version>_amd64.deb`), then open clawdyX from the applications menu.
{{- end}}

## First run

The setup wizard asks for the bot token, finds your Telegram ID from a message you send to the bot, checks Claude Code, lets you pick a project folder, then installs and starts the worker. The token is stored in {{.W.TokenStore}}.{{if .Linux}} Without a keyring (common on servers) it goes into a private file instead.{{end}}

Then open Telegram and send `/start` to your bot. Any plain message is a prompt for Claude.

## Where things are

| What | Where |
| --- | --- |
| Settings | `{{.Paths.Config}}` (change them in Settings, not by hand) |
| State (sessions, jobs, audit) | `{{.Paths.State}}` |
| Logs | `{{.Paths.Logs}}` |
| The worker | `{{.Paths.Worker}}` |
| The token | {{.W.TokenStore}} |

## Moving around the app

{{if .Darwin}}⌘1–8{{else}}Ctrl+1–8{{end}} switch screens, {{if .Darwin}}⌘R{{else}}Ctrl+R{{end}} refreshes, {{if .Darwin}}⌘,{{else}}Ctrl+,{{end}} opens Settings, and `?` opens this Help.
