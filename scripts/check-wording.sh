#!/bin/sh
# Fails when user-facing text tells people to edit config.json: the app
# edits every setting. Code that only names or reports the path is fine
# (tests, path helpers, the string "config.json" itself, comments), and
# ARCHITECTURE.md documents the file format on purpose.
set -eu
hits=$(git grep -nI 'config\.json' -- internal cmd desktop/*.go desktop/host desktop/frontend/src README.md docs \
	| grep -v -E '_test\.go:|desktop/frontend/src/__tests__/|DefaultPath|StandardPath|legacyConfigPath|"config\.json"|//[^"]*config\.json|^docs/ARCHITECTURE\.md:' \
	| grep -iE 'edit config.json|in (~/.config/clawdyx/)?config.json' || true)
if [ -n "$hits" ]; then
	echo "$hits"
	echo "user-facing text tells people to edit config.json; point at the app's Settings instead"
	exit 1
fi
echo "no config.json instructions found"
