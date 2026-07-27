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

# The website carries two compiled constants. Latest is the newest release and
# moves every time, so it must already name this version before the release
# exists. Required is the OLDEST supported build and usually does not move at
# all; it only has to not be newer than what we are cutting, which would mean
# the site refuses the very release being published.
#
# Checked rather than trusted to memory, because the failure is silent until
# somebody tries to use the CLI. Set SKIP_SITE_CHECK=1 to release anyway.
SITE="${STACKDRIFT_URL:-https://stackdrift.net}"
if [ -z "${SKIP_SITE_CHECK:-}" ]; then
  echo "==> Checking $SITE already expects v$NEW"
  site_json="$(curl -fsS "$SITE/api/cli/version" 2>/dev/null || true)"
  site_latest="$(printf '%s' "$site_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('latestVersion',''))" 2>/dev/null || true)"
  site_requires="$(printf '%s' "$site_json" | python3 -c "import sys,json; print(json.load(sys.stdin).get('requiredVersion',''))" 2>/dev/null || true)"

  newer() { python3 -c "
import sys
def parse(v): return [int(p) for p in v.strip().lstrip('v').split('.') if p.isdigit()]
a, b = parse(sys.argv[1]), parse(sys.argv[2])
a += [0] * (len(b) - len(a)); b += [0] * (len(a) - len(b))
sys.exit(0 if a > b else 1)
" "$1" "$2"; }

  if [ -z "$site_latest" ] && [ -z "$site_requires" ]; then
    echo "    could not ask $SITE which version it expects; continuing" >&2
  elif [ -z "$site_latest" ]; then
    echo "    $SITE does not report latestVersion yet, so it predates the Required/Latest split." >&2
    echo "    Deploy the website first, then re-run this. (SKIP_SITE_CHECK=1 overrides.)" >&2
    exit 1
  elif [ "$site_latest" != "$NEW" ]; then
    echo "    $SITE says the latest release is $site_latest, not $NEW." >&2
    echo "    Set CliVersions.Latest to $NEW and deploy the website first, then re-run this." >&2
    echo "    (SKIP_SITE_CHECK=1 overrides.)" >&2
    exit 1
  elif newer "$site_requires" "$NEW"; then
    echo "    $SITE requires at least $site_requires, which is newer than $NEW." >&2
    echo "    The site would refuse the release being cut. Fix CliVersions.Required first." >&2
    exit 1
  else
    echo "    ok (latest $site_latest, minimum supported $site_requires)"
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

