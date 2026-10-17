#!/bin/sh
# Assembles bin/clawdyX.app from the built pieces and signs it inside-out.
#   bin/clawdyX-gui          desktop app (Wails)          -> Contents/MacOS/clawdyX
#   bin/clawdyx              worker (CGO-free)            -> Contents/Helpers/clawdyx
#   bin/clawdyx-screenshot   ScreenCaptureKit helper      -> Contents/Helpers/clawdyx-screenshot
# CLAWDYX_SIGN_IDENTITY selects the signing identity (default: ad-hoc "-").
# CLAWDYX_HARDENED=1 adds the hardened runtime (needed for notarization).
set -eu
VERSION=${VERSION:-0.0.0}
APP=bin/clawdyX.app
IDENT=${CLAWDYX_SIGN_IDENTITY:--}
OPTS=""
[ "${CLAWDYX_HARDENED:-0}" = "1" ] && OPTS="--options runtime --timestamp"

for f in bin/clawdyX-gui bin/clawdyx bin/clawdyx-screenshot; do
  [ -x "$f" ] || { echo "missing $f; run make app" >&2; exit 1; }
done
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Helpers" "$APP/Contents/Resources"
cp bin/clawdyX-gui "$APP/Contents/MacOS/clawdyX"
cp bin/clawdyx "$APP/Contents/Helpers/clawdyx"
cp bin/clawdyx-screenshot "$APP/Contents/Helpers/clawdyx-screenshot"
cp desktop/build/icons.icns "$APP/Contents/Resources/icons.icns"
sed "s/__VERSION__/$VERSION/g" desktop/build/Info.plist > "$APP/Contents/Info.plist"
cp LICENSE "$APP/Contents/Resources/LICENSE.txt"
[ -f THIRD_PARTY_NOTICES.md ] && cp THIRD_PARTY_NOTICES.md "$APP/Contents/Resources/THIRD_PARTY_NOTICES.md"
xattr -cr "$APP"

# Sign nested code first, each with its own identifier, then the app.
# shellcheck disable=SC2086
codesign --force $OPTS --identifier io.github.publicthrone547.clawdyx --sign "$IDENT" "$APP/Contents/Helpers/clawdyx"
# shellcheck disable=SC2086
codesign --force $OPTS --identifier io.github.publicthrone547.clawdyx.screenshot --sign "$IDENT" "$APP/Contents/Helpers/clawdyx-screenshot"
# shellcheck disable=SC2086
codesign --force $OPTS --identifier io.github.publicthrone547.clawdyx.app --sign "$IDENT" "$APP"
codesign --verify --strict --deep "$APP"
if [ "$IDENT" = "-" ]; then
  echo "built $APP (ad-hoc signed: macOS grants such as Screen Recording reset with each build; set CLAWDYX_SIGN_IDENTITY to keep them)"
else
  echo "built $APP (signed with $IDENT)"
fi
du -sh "$APP"
