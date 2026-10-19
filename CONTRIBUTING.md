# Contributing

clawdyX is a small project with one maintainer. Bug reports and focused pull requests are welcome; security problems go through the private process in [`SECURITY.md`](SECURITY.md), never a public issue. The license is the PolyForm Noncommercial License 1.0.0 ([`LICENSE`](LICENSE)); by contributing you agree that your contribution is licensed under it.

## Rules

- **Never add Claude as a contributor or author.** This covers `CONTRIBUTORS`, `AUTHORS`, README, `package.json`, `go.mod`, `info.json`, `Info.plist`, the About screen, release notes and anything else. Commit messages must not contain `Co-Authored-By: Claude …`, `Claude-Session: …`, `🤖 Generated with Claude Code` or any similar trailer, even if tooling adds one by default. Strip such lines and remove any that already exist.
- **Small commits with Conventional Commits:** `feat:`, `fix:`, `refactor:`, `docs:`, `build:`, `test:`, `chore:`, with a scope where it helps (`feat(screen): …`). The body says why, in plain sentences.
- **The platform layer.** Nothing outside `internal/platform` branches on `runtime.GOOS`; code that differs per OS lives in files with build tags (`_darwin.go`, `_windows.go`, `_linux.go`, `_unix.go` for `!windows`) behind a shared interface or function. `TestNoGOOSOutsidePlatform` enforces it. A test file that only runs on one OS needs the OS in its name (`foo_windows_test.go`; `windows_test.go` alone has no constraint).
- **The wording rule.** User-facing text (bot replies, the CLI, prompts for Claude, the app, Help, the docs' per-OS sections) names no other operating system. OS words come from `platform.W` (Go) or `words` (the app), defined once per OS in `internal/platform/words.go`. The guards (`internal/platform/forbidden.json`, the source scan, the rendered-reply test, the Vitest views test, the Help test, `make check-docs`) fail otherwise. After changing `words.go` or `forbidden.json`, run `go test ./internal/platform -update-fixtures` so the app's test fixtures follow.
- **Fail closed, no new network destinations.** An invalid setting stops the worker; clawdyX talks only to Telegram's Bot API, the local Claude Code, and GitHub when the owner clicks "Check for updates".
- **Dependencies:** MIT, BSD, ISC, Apache-2.0 or MPL-2.0 only; run `scripts/gen-notices.sh` and commit `THIRD_PARTY_NOTICES.md` with the change.
- **No secrets in the repository:** no tokens, Telegram IDs, transcripts, screenshots of a real desktop, state databases or logs. `make check-secrets` scans for token-shaped strings.

## Building and testing

Every OS needs Go (the version in `go.mod`) and Node.js 22 or newer.

| OS | Also needed | Build | Notes |
| --- | --- | --- | --- |
| macOS | Xcode command-line tools | `make app`, `make dmg` | the app bundles the worker and the Swift capture and camera helpers |
| Windows | Git for Windows (its bash runs the Makefile), GNU make; Visual Studio Build Tools with the C++ workload for the camera helper; NSIS for the installer | `make build-windows`, `make installer-windows` | without gcc, `make test` runs without `-race` |
| Linux | `build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev` (see `desktop/build/linux/README.md` for other distributions) | `make app-linux`, `make package-linux` | the app needs cgo; the worker cross-compiles from anywhere (`make build-linux`) |

`make check-all` runs the Go tests, vet, gofmt, the secret scan, the wording and docs guards, the desktop module's tests, the frontend type check and Vitest, and the type checks of both modules for all three OSes. It must be green before a pull request. `make help` lists every target.

Test without touching a running bot: use a separate settings file (`CLAWDYX_CONFIG`), a separate service name (`CLAWDYX_SERVICE_LABEL=local.clawdyx.dev`) and a separate test bot token; see `docs/TESTING.md`.

## Adding a setting

A setting lives in one place per layer:

1. `internal/config`: the field with its JSON key, its default in `applyDefaults`, its validation in `validate` (fail closed).
2. `internal/control/api.go`: the field in `SettingsInfo` and a pointer field in `SettingsPatch`, and the section constant it belongs to.
3. `internal/settings`: `Info` fills it, `Apply` changes it (only what `config.Parse` cannot check), `Reset` restores it for its section. The worker's `settings.*` methods and the app's offline editing both use this package, and every change goes through `config.Rewrite`, which validates the result exactly like the worker at startup.
4. The app: the field in the section's view under `desktop/frontend/src/views/settings/`, with OS words from `words` where it names something of the OS. Regenerate the bindings (`make bindings`).
5. Tests: `internal/settings` (apply, reset, export/import), the section's Vitest case, and the docs (README's settings line if users need to know).

## Adding a Help topic

1. Write `desktop/help/<id>.md`. It is a Go `text/template` first: use `{{.W.Field}}` for OS words, `{{if .Darwin}}…{{else if .Windows}}…{{else}}…{{end}}` for OS-specific parts, `{{.Session}}` for the Linux session kind (`x11`, `wayland`, `headless`) and `{{.Paths.Config}}`, `.State`, `.Logs`, `.Worker` for this computer's paths. Then Markdown (tables and strikethrough; raw HTML is dropped).
2. Links are `[text](help:<other-id>)` or `[text](open:<key>)` with a key of the app's link list (`botfather`, `releases`, `docs`, and the settings pages of that OS); nothing else is followed.
3. Add the topic to `helpTopics` in `desktop/help.go`.
4. `go test ./desktop -run TestHelp` renders it for every OS and Linux session and checks forbidden words, links and render time.

## Releases

The version has one source: `VERSION` in the Makefile. The release process is in [`docs/RELEASE.md`](docs/RELEASE.md); pushing tags and publishing releases is the maintainer's manual step.
