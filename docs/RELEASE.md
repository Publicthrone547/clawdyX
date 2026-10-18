# Release notes and checklist

## Measured baseline (v0.3.0)

Measured on 2026-10-17 on the development Mac: macOS 27.0.1, Apple M3 Max, 36 GB, two external displays (3440×1440 and 1920×1080), Claude Code 2.1.292, app installed in `/Applications`, worker under launchd, no job running.

| Metric | Budget | Measured | Notes |
| --- | --- | --- | --- |
| `clawdyX.app` size | ≤ 45 MB | 29 MB | app 11 MB, worker 19 MB, capture helper 85 KB, icons |
| `.dmg` size | ≤ 30 MB | 15 MB | `make dmg`, UDZO |
| Worker idle memory | ≤ 50 MB | 28–29 MB RSS | after 90 s idle, polling Telegram |
| Worker idle CPU | ~0 | 0.00 s CPU over 10 s | long polling, 30 s timeout |
| App cold start to first window | ≤ 1.5 s | 0.48–0.60 s | three runs, includes ~0.1 s measuring overhead |
| App memory, window open | ≤ 60 MB (app process) | 120–127 MB app + ~91 MB WebKit helpers | **over budget**, see below |
| App memory, window closed | ≤ 60 MB | 135–138 MB app + the same WebKit helpers | **over budget**; closing hides the window |
| Control socket round trip (`status.get`) | p50 ≤ 5 ms | p50 0.65 ms, p95 1.2 ms | 20 calls on one connection |
| Capture one display (helper) | ≤ 1.5 s to preview | 0.21 s (1920×1080), 0.32 s (3440×1440) + 0.09 s re-encode | PNG 0.25 MB / 5.1 MB in memory |
| Telegram reply budget | 1 msg/s per chat | as designed | Bot API guidance |

**App memory.** A WKWebView-based app (Wails) carries WebKit's frameworks in its own process plus WebKit's GPU, content and networking processes. Destroying the window on close was tried: WebKit keeps its helper processes alive for reuse and the app process did not shrink, so it saved nothing and made reopening slower. The window is hidden instead. The bot does not depend on the app: quitting it (menu-bar item → Quit) leaves the 29 MB worker running and frees the rest.

How to measure again:

```sh
ps -o rss=,pcpu= -p $(pgrep -x clawdyx)                                  # worker
ps -o rss=,pcpu= -p $(pgrep -f '/Applications/clawdyX.app/Contents/MacOS/clawdyX')
ps -axo pid,rss,comm | grep WebKit                                       # WebKit helper processes
go test ./internal/screen/ -run XXX -bench EncodeLargeDisplay            # re-encode cost
```

## Release checklist

1. `make check-all` is green (Go race tests, vet, gofmt, secret scan, desktop Go tests, frontend type check and Vitest, Windows type check).
2. `scripts/gen-notices.sh` regenerated `THIRD_PARTY_NOTICES.md`; review new licenses (anything not MIT/BSD/ISC/Apache/MPL needs a decision).
3. Version: `VERSION` in the Makefile, `version` in `cmd/clawdyx/main.go` and `desktop/main.go`, `desktop/frontend/package.json` and `package-lock.json`, `CHANGELOG.md`. (The Windows file version comes from `VERSION` via `desktop/build/windows/info.json`.)
4. Build: `make release` runs `check-all`, `make dmg` and `make build-windows` and writes `bin/release-<version>/` (dmg, darwin tar.gz, Windows zip, `SHA256SUMS`); add the release notes as `NOTES.md` there, with absolute links. For distribution on macOS, sign and notarize: `CLAWDYX_SIGN_IDENTITY="Developer ID Application: … (TEAMID)" CLAWDYX_HARDENED=1 make dmg`, then `xcrun notarytool submit bin/clawdyX-<v>.dmg --keychain-profile <profile> --wait` and `xcrun stapler staple bin/clawdyX-<v>.dmg`. Never commit signing material.
5. Smoke test the built app: wizard on an unconfigured account, Start/Stop, a Telegram prompt, a capture (with permission), Quit dialog, reopen.
6. Privacy scan of everything that will be published (review every hit by hand; a clean scan is not a guarantee):

   ```sh
   make check-secrets
   git grep -n -i -E '<your telegram id>|<your macOS user name>|<your e-mail>' -- .
   git grep -n -E '/Users/[a-z0-9_-]+/' -- . | grep -v -E '/Users/(you|<you>|me)/'
   git ls-files | grep -i -E 'analyst|brief|private|\.env$|\.db$|\.log$|\.png$|\.jpg$' | grep -v -E 'desktop/build/(appicon|tray-template)\.png'
   git log --all --format='%H %s' | head -50                 # only reviewed commits
   git log --all -p | grep -i -E '<your telegram id>|<your e-mail>'   # history, not just the tree
   ```

7. Tag locally: `git tag -a v<version> -m "clawdyX <version>"`. Pushing (and publishing a release) is the owner's manual step.
