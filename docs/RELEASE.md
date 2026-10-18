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

| File | Built by | Contents |
| --- | --- | --- |
| `clawdyX-<v>-darwin-arm64.dmg` | `make release-darwin` (macOS) | `clawdyX.app` with the worker and the capture and camera helpers |
| `clawdyx-<v>-darwin-arm64.tar.gz` | the same | the worker, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `README.md` |
| `clawdyX-<v>-windows-amd64-setup.exe` | `make release-windows` (Windows) | the per-user NSIS installer |
| `clawdyX-<v>-windows-amd64.zip` | the same | `clawdyX/` with `clawdyX.exe`, `helpers\clawdyx.exe`, `clawdyxw.exe`, `clawdyx-camera.exe` and the license files |
| `clawdyx_<v>_amd64.deb`, `clawdyx-<v>-1.x86_64.rpm` | `make release-linux` (Linux) | the package `clawdyx` (nfpm) |
| `clawdyX-<v>-linux-amd64.tar.gz` | the same | `clawdyX`, `clawdyx`, `clawdyX.desktop`, `icons/`, the license files |
| `clawdyX-<v>-linux-amd64.AppImage` | `make appimage-linux` (Linux, best effort) | the app and worker; WebKitGTK from the host |
| `clawdyx-<v>-linux-amd64.tar.gz`, `clawdyx-<v>-linux-arm64.tar.gz` | `make release-workers` (anywhere) | the worker and the license files |
| arm64 Linux packages and app tarball | the release workflow, when the `ubuntu-24.04-arm` runner exists | as for amd64 |
| `SHA256SUMS` | `make release-sums`, and the workflow's publish job | every file above |

The Windows camera helper uses only the Windows SDK (Media Foundation), so it adds no third-party notice. nfpm (MIT), linuxdeploy and its GTK plugin (MIT) and NSIS (zlib) are build tools; nothing of them is linked into clawdyX.

## Version

`VERSION` in the Makefile is the only source. Every build passes it with `-X main.version=…` (both programs default to `0.0.0-dev`), and the build files take it from there: `Info.plist`, the Windows `info.json` and manifest (`__VERSION__`), nfpm and NSIS (`VERSION`). `desktop/frontend/package.json` stays `0.0.0`. `make check-version` (part of `check-all`, and the first job of the release workflow with `TAG=v<version>`) fails unless `CHANGELOG.md` starts with `## <version>` and `desktop/build/release-notes/<version>.md` exists.

## Release checklist

1. `make check-all` is green on the development machine, and CI is green on macOS, Windows and Linux for the release commit.
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

6. Tag locally: `git tag -a v<version> -m "clawdyX <version>"`. Pushing the tag (`git push origin main v<version>`) is the owner's manual step; it starts the release workflow.
7. The workflow builds every file on its own system and creates a **draft** release with `SHA256SUMS`. Download the files, check them (`sha256sum -c SHA256SUMS --ignore-missing`, `shasum -a 256 -c SHA256SUMS --ignore-missing` on macOS, `Get-FileHash` in PowerShell), run the QA checklists, then publish the draft by hand.
8. After publishing: fill in the SHA-256 of the installer and the zip in `packaging/winget/…/<version>/` and `packaging/scoop/clawdyx.json` from `SHA256SUMS`, run `winget validate --manifest <folder>`, and open the pull request to microsoft/winget-pkgs; copy the Scoop manifest into the Scoop bucket repository.

Without the workflow, each system builds its own files with `make release` (it runs `check-all`, builds that system's files and the Linux worker tarballs, and writes `SHA256SUMS` for them); collect them into one folder and run `make release-sums`.

## Signing (optional)

The workflow signs only when its secrets exist; without them it builds unsigned files and says so in the job summary.

| System | Secrets | What happens |
| --- | --- | --- |
| macOS | `MACOS_CERT_P12` (base64 of a Developer ID Application certificate), `MACOS_CERT_PASSWORD` | the app and its helpers are signed with the hardened runtime (`CLAWDYX_SIGN_IDENTITY`, `CLAWDYX_HARDENED=1`) |
| macOS | `APPLE_ID`, `APPLE_TEAM_ID`, `APPLE_APP_PASSWORD` (with the certificate) | the dmg is notarized (`xcrun notarytool submit --wait`) and stapled |
| Windows | `AZURE_KEY_VAULT_URL`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_TENANT_ID`, `AZURE_CERT_NAME` | `clawdyX.exe`, the helpers and the installer are signed with AzureSignTool (Authenticode, timestamped) |

Locally on a Mac: `CLAWDYX_SIGN_IDENTITY="Developer ID Application: … (TEAMID)" CLAWDYX_HARDENED=1 make dmg`, then `xcrun notarytool submit bin/clawdyX-<v>-darwin-arm64.dmg --keychain-profile <profile> --wait` and `xcrun stapler staple bin/clawdyX-<v>-darwin-arm64.dmg`. Never commit signing material. Unsigned files need **Open Anyway** on macOS and **More info → Run anyway** on Windows; the README says so.
