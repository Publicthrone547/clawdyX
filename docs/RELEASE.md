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

The macOS figures are from the development Mac; Windows and Linux have not been measured yet (record them with the release QA).

## Release files

macOS and Windows are released; Linux is not released for now (the code builds, see `desktop/build/linux/README.md`). Everything is built on the Mac with `make release`.

| File | Contents |
| --- | --- |
| `clawdyX-<v>-darwin-arm64.dmg` | `clawdyX.app` with the worker and the capture and camera helpers |
| `clawdyx-<v>-darwin-arm64.tar.gz` | the worker, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `README.md` |
| `clawdyX-<v>-windows-amd64-setup.exe` | the per-user NSIS installer |
| `clawdyX-<v>-windows-amd64.zip` | `clawdyX/` with `clawdyX.exe`, `helpers\clawdyx.exe`, `clawdyxw.exe`, `clawdyx-camera.exe` and the license files |
| `SHA256SUMS` | every file above |

The Windows programs cross-compile on the Mac. The Windows camera helper (C++, Media Foundation) needs MSVC: build it on the Windows PC with `make helper-windows` and copy `bin\windows\clawdyX\helpers\clawdyx-camera.exe` to `bin/clawdyx-camera.exe` on the Mac; `make release` stops without it. The installer needs NSIS on the Mac (`brew install makensis`).

The Windows camera helper uses only the Windows SDK (Media Foundation), so it adds no third-party notice. NSIS (zlib) is a build tool; nothing of it is linked into clawdyX.

## Version

`VERSION` in the Makefile is the only source. Every build passes it with `-X main.version=…` (both programs default to `0.0.0-dev`), and the build files take it from there: `Info.plist`, the Windows `info.json` and manifest (`__VERSION__`), nfpm and NSIS (`VERSION`). `desktop/frontend/package.json` stays `0.0.0`. `make check-version` (part of `check-all`; `make check-version TAG=v<version>` also checks a tag) fails unless `CHANGELOG.md` starts with `## <version>` and `desktop/build/release-notes/<version>.md` exists.

## Release checklist

1. `make check-all` is green on the Mac and on the Windows PC for the release commit.
2. `scripts/gen-notices.sh` regenerated `THIRD_PARTY_NOTICES.md` (`make check-notices` compares); review new licenses (anything not MIT/BSD/ISC/Apache/MPL needs a decision).
3. Version: `VERSION` in the Makefile, the `CHANGELOG.md` entry dated on release day, `desktop/build/release-notes/<version>.md` (the release page text, with a "Tested on" table from the QA checklists in `docs/TESTING.md`), the winget and Scoop manifests' version (`packaging/`).
4. `make check-attribution`: no commit names Claude as an author or co-author.
5. Privacy scan of everything that will be published (review every hit by hand; a clean scan is not a guarantee):

   ```sh
   make check-secrets
   git grep -n -i -E '<your telegram id>|<your user name>|<your e-mail>' -- .
   git grep -n -E '/Users/[a-z0-9_-]+/|/home/[a-z0-9_-]+/|C:\\Users\\[A-Za-z0-9_-]+' -- . | grep -v -E '/(you|<you>|me)/|Users\\you'
   git ls-files | grep -i -E 'analyst|brief|private|plan|progress|\.env$|\.log$|\.png$|\.jpg$' | grep -v -E 'desktop/build/(appicon|tray-template)\.png|desktop/build/linux/icons/'
   git log --all --format='%H %s' | head -60                 # only reviewed commits
   git log --all -p | grep -i -E '<your telegram id>|<your e-mail>'   # history, not just the tree
   ```

6. On the Mac: `make release` (the camera helper in `bin/clawdyx-camera.exe`, see above). It runs `check-all` and writes the files and `SHA256SUMS` to `bin/release-<version>/`. Run the QA checklists in `docs/TESTING.md` with these files.
7. Tag locally: `git tag -a v<version> -m "clawdyX <version>"`. The owner pushes `main` and the tag and creates the release with the files: `gh release create v<version> bin/release-<version>/* --title "clawdyX <version>" --notes-file desktop/build/release-notes/<version>.md`.
8. After publishing: fill in the SHA-256 of the installer and the zip in `packaging/winget/…/<version>/` and `packaging/scoop/clawdyx.json` from `SHA256SUMS`, run `winget validate --manifest <folder>`, and open the pull request to microsoft/winget-pkgs; copy the Scoop manifest into the Scoop bucket repository.

## Signing (optional)

The files are not signed yet. On the Mac: `CLAWDYX_SIGN_IDENTITY="Developer ID Application: … (TEAMID)" CLAWDYX_HARDENED=1 make dmg`, then `xcrun notarytool submit bin/clawdyX-<v>-darwin-arm64.dmg --keychain-profile <profile> --wait` and `xcrun stapler staple bin/clawdyX-<v>-darwin-arm64.dmg`. Windows files would be signed with an Authenticode certificate (for example AzureSignTool, timestamped) before the installer is built. Never commit signing material. Unsigned files need **Open Anyway** on macOS and **More info → Run anyway** on Windows; the README says so.
