#!/bin/sh
# Fails when a commit names Claude as an author or co-author (commit
# trailers, author or committer fields). Checks the commits not yet on
# BASE (default origin/main), or the whole history when BASE is missing.
set -eu
base=${BASE:-origin/main}
if git rev-parse --verify -q "$base" >/dev/null; then range="$base..HEAD"; else range=HEAD; fi
pattern='co-authored-by: *claude|generated with .*claude|claude-session|noreply@anthropic\.com'
bad=$(git log --format='%H %an <%ae> %cn <%ce>%n%B' "$range" | grep -inE "$pattern" || true)
if [ -n "$bad" ]; then
	echo "$bad"
	echo "commits in $range name Claude as an author or co-author; remove those lines (CONTRIBUTING.md)"
	exit 1
fi
echo "no Claude attribution in $range"
