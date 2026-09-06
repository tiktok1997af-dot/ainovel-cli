#!/bin/sh
# Deterministic release notes from Git history only.
# Usage: .github/scripts/gen-changelog.sh [previous_tag]
#
# This script intentionally performs no AI/API/network calls and consumes no
# AI credentials. Release notes therefore remain reproducible from repository
# history and cannot reintroduce an API-era execution path.
set -eu

PREV_TAG="${1:-$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || true)}"
CURR_TAG="$(git describe --tags --abbrev=0 HEAD 2>/dev/null || printf '%s' HEAD)"

if [ -n "$PREV_TAG" ]; then
    RANGE="${PREV_TAG}..${CURR_TAG}"
    COMMITS="$(git log "$RANGE" --pretty=format:'- %s' --no-merges)"
else
    RANGE="last 50 commits"
    COMMITS="$(git log "$CURR_TAG" --pretty=format:'- %s' --no-merges -50)"
fi

printf '## What changed\n\n'
if [ -n "$COMMITS" ]; then
    printf '%s\n' "$COMMITS"
else
    printf '%s\n' "- No non-merge commits found in ${RANGE}."
fi
