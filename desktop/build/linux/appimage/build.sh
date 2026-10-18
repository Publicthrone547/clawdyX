#!/bin/sh
# Builds clawdyX-VERSION-linux-ARCH.AppImage from the files app-linux writes
# to bin/linux/clawdyX/, with linuxdeploy and its GTK plugin (both MIT,
# https://github.com/linuxdeploy). WebKitGTK is not bundled (its helper
# processes are found by fixed paths), so the AppImage needs the host's
# libwebkit2gtk-4.1, and on Ubuntu 22.04 and later libfuse2 to start.
#
#   desktop/build/linux/appimage/build.sh 1.0.0 amd64
set -eu
version=$1
arch=${2:-amd64}
case $arch in
amd64) la=x86_64 ;;
arm64) la=aarch64 ;;
*) echo "unknown architecture $arch" >&2; exit 2 ;;
esac
src=bin/linux/clawdyX
work=bin/appimage
out=${OUT:-bin/release-$version}
rm -rf "$work" && mkdir -p "$work/AppDir/usr/bin" "$work/tools" "$out"
cp "$src/clawdyX" "$src/clawdyx" "$work/AppDir/usr/bin/"

tools=$work/tools
base=https://github.com/linuxdeploy
curl -fsSL -o "$tools/linuxdeploy" "$base/linuxdeploy/releases/download/continuous/linuxdeploy-$la.AppImage"
curl -fsSL -o "$tools/linuxdeploy-plugin-gtk.sh" "https://raw.githubusercontent.com/linuxdeploy/linuxdeploy-plugin-gtk/master/linuxdeploy-plugin-gtk.sh"
chmod +x "$tools/linuxdeploy" "$tools/linuxdeploy-plugin-gtk.sh"

PATH="$tools:$PATH" \
	DEPLOY_GTK_VERSION=3 \
	APPIMAGE_EXTRACT_AND_RUN=1 \
	LDAI_OUTPUT="$out/clawdyX-$version-linux-$arch.AppImage" \
	ARCH=$la \
	"$tools/linuxdeploy" --appdir "$work/AppDir" \
	--executable "$work/AppDir/usr/bin/clawdyX" \
	--desktop-file desktop/build/linux/clawdyX.desktop \
	--icon-file desktop/build/linux/icons/hicolor/256x256/apps/clawdyx.png \
	--exclude-library 'libwebkit2gtk-4.1.so*' \
	--exclude-library 'libjavascriptcoregtk-4.1.so*' \
	--plugin gtk \
	--output appimage
ls -lh "$out/clawdyX-$version-linux-$arch.AppImage"
