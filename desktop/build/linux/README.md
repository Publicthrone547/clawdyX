# Building the Linux app

The desktop app needs cgo and GTK 3 with WebKitGTK 4.1 (`-tags production,gtk3`),
so it is built on Linux. The
worker (`clawdyx`) needs neither and cross-compiles from any system
(`make build-linux`).

Build dependencies:

| Distribution | Packages |
| --- | --- |
| Ubuntu 22.04+, Debian 12+ | `sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libx11-dev` |
| Fedora 40+ | `sudo dnf install gcc pkgconf-pkg-config gtk3-devel webkit2gtk4.1-devel libX11-devel` |
| Arch | `sudo pacman -S base-devel gtk3 webkit2gtk-4.1 libx11` |

Plus Go (the version in `go.mod`) and Node.js 22 or newer. Then:

```bash
make app-linux    # bin/linux/clawdyX/: clawdyX (app), clawdyx (worker), clawdyX.desktop, icons/
make package-linux # bin/release-<version>/: .deb, .rpm (nfpm) and clawdyX-<version>-linux-<arch>.tar.gz
make appimage-linux # bin/release-<version>/clawdyX-<version>-linux-<arch>.AppImage (best effort)
```

Files in this folder:

- `clawdyX.desktop` — the launcher entry (installed to `/usr/share/applications`).
- `icons/hicolor/*/apps/clawdyx.png` — the app icon in the hicolor theme sizes,
  generated from `desktop/build/appicon.png` with `go run scripts/linux-icons.go`.
- `nfpm.yaml` — the .deb and .rpm (package `clawdyx`: `/usr/bin/clawdyX`, `/usr/bin/clawdyx`,
  the launcher entry, the icons, the license files). nfpm (MIT) runs through `go run`.
- `appimage/build.sh` — the AppImage, with linuxdeploy and its GTK plugin (MIT). WebKitGTK
  stays on the host, so the AppImage needs `libwebkit2gtk-4.1` installed, and `libfuse2` on
  Ubuntu 22.04 and later.

Without a package, install the launcher entry and the icons of the tar.gz for your user:
`xdg-desktop-menu install --novendor clawdyX.desktop` and
`xdg-icon-resource install --novendor --size 256 icons/hicolor/256x256/apps/clawdyx.png clawdyx`.

Running the app needs the runtime libraries only: `libgtk-3-0` and
`libwebkit2gtk-4.1-0` (Debian/Ubuntu), `gtk3` and `webkit2gtk4.1` (Fedora).
The tray icon needs a StatusNotifier host: KDE has one; on GNOME install the
AppIndicator extension (without it, closing the window quits the app; the
worker keeps running).
