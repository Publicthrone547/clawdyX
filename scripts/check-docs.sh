#!/bin/sh
# Fails when a per-OS section of the user docs (README, RUNBOOK, PRIVACY)
# names another operating system or its products. The check itself is a Go
# test so it shares the forbidden-word lists of the app's wording guards.
set -eu
exec go test -count=1 -run 'TestDocsOSSectionsNameOnlyTheirOS|TestOSSections' ./internal/platform
