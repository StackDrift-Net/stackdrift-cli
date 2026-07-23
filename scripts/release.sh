#!/usr/bin/env bash
set -euo pipefail

# Cuts a GitHub release for the StackDrift CLI. Bumps the version (patch by
# default, or pass an explicit version as the first argument), rebuilds the
# binaries, commits, tags, pushes, creates the GitHub release and uploads the
# six platform binaries as assets. The CLI's update command compares its own
# version against the latest release published here.
#
# Auth: a GitHub token with contents:write on the repo, taken from the GH_TOKEN
# environment variable or, if unset, from the file at STACKDRIFT_GH_TOKEN_FILE
# (default ~/.config/stackdrift/gh-token). The token is never committed.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="digitalaffinity-au/stackdrift-cli"
TOKEN_FILE="${STACKDRIFT_GH_TOKEN_FILE:-$HOME/.config/stackdrift/gh-token}"

cd "$ROOT"

TOKEN="${GH_TOKEN:-}"
if [ -z "$TOKEN" ] && [ -f "$TOKEN_FILE" ]; then
  TOKEN="$(tr -d ' \t\n\r' < "$TOKEN_FILE")"
fi
if [ -z "$TOKEN" ]; then
  echo "No GitHub token. Set GH_TOKEN or write one to $TOKEN_FILE" >&2
  exit 1
fi

CURRENT="0.0.0"
[ -f VERSION ] && CURRENT="$(tr -d ' \t\n\r' < VERSION)"

if [ "${1:-}" != "" ]; then
  NEW="$1"
else
  NEW="$(python3 -c "p='$CURRENT'.split('.'); p[2]=str(int(p[2])+1); print('.'.join(p))")"
fi

echo "==> Releasing v$NEW (current $CURRENT)"

# The website carries the required CLI version as a compiled constant, so it has
# to be deployed with this version BEFORE the release exists. Get that backwards
# and every CLI is refused while being told to upgrade to something it cannot
# download yet.
#
# Checked rather than trusted to memory, because the failure is silent until
# somebody tries to use the CLI. Set SKIP_SITE_CHECK=1 to release anyway.
SITE="${STACKDRIFT_URL:-https://stackdrift.net}"
if [ -z "${SKIP_SITE_CHECK:-}" ]; then
  echo "==> Checking $SITE already expects v$NEW"
  site_requires="$(curl -fsS "$SITE/api/cli/version" 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['requiredVersion'])" 2>/dev/null || true)"

  if [ -z "$site_requires" ]; then
    echo "    could not ask $SITE which version it requires; continuing" >&2
  elif [ "$site_requires" != "$NEW" ]; then
    echo "    $SITE requires $site_requires, not $NEW." >&2
    echo "    Bump CliVersions.Required to $NEW and deploy the website first, then re-run this." >&2
    echo "    (SKIP_SITE_CHECK=1 overrides.)" >&2
    exit 1
  else
    echo "    ok"
  fi
fi

if git rev-parse "v$NEW" >/dev/null 2>&1; then
  echo "Tag v$NEW already exists" >&2
  exit 1
fi

echo "$NEW" > VERSION
bash scripts/build.sh "$NEW"

git add -A
if ! git diff --cached --quiet; then
  git commit -q -m "Release v$NEW"
fi

echo "==> Syncing with remote"
GIT_SSH_COMMAND='ssh -o BatchMode=yes' git fetch origin
GIT_SSH_COMMAND='ssh -o BatchMode=yes' git rebase origin/main

git tag "v$NEW"
GIT_SSH_COMMAND='ssh -o BatchMode=yes' git push origin main
GIT_SSH_COMMAND='ssh -o BatchMode=yes' git push origin "v$NEW"

echo "==> Creating GitHub release"
payload="$(python3 -c "import json,sys; v=sys.argv[1]; print(json.dumps({'tag_name':'v'+v,'name':'v'+v,'draft':False,'prerelease':False,'generate_release_notes':True}))" "$NEW")"
resp="$(curl -fsSL -X POST \
  -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.github+json" \
  "https://api.github.com/repos/$REPO/releases" \
  -d "$payload")"
release_id="$(printf '%s' "$resp" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")"

for f in dist/stackdrift-*; do
  name="$(basename "$f")"
  echo "==> Uploading $name"
  curl -fsSL -X POST \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/octet-stream" \
    --data-binary @"$f" \
    "https://uploads.github.com/repos/$REPO/releases/$release_id/assets?name=$name" >/dev/null
done

echo "==> Released: https://github.com/$REPO/releases/tag/v$NEW"

