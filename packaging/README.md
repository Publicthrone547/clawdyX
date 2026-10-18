# Package manager manifests

Prepared for each release; the owner submits them after publishing it,
because both need the published download and its SHA-256 (from the
release's `SHA256SUMS`). Steps: `docs/RELEASE.md`.

- `winget/Publicthrone547.clawdyX/<version>/` — the per-user installer
  (`nullsoft`, `/S`). Replace `InstallerSha256`, check with
  `winget validate --manifest <folder>`, then open a pull request to
  microsoft/winget-pkgs.
- `scoop/clawdyx.json` — the `.zip`, with `clawdyx` on PATH and a
  clawdyX shortcut. Replace `hash`; it belongs in a Scoop bucket (a
  separate repository). `checkver`/`autoupdate` follow later releases.
