#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI (gh) is required. Install it from https://cli.github.com/ and run: gh auth login" >&2
  exit 2
fi
if ! command -v git >/dev/null 2>&1; then
  echo "Git is required." >&2
  exit 2
fi
if ! gh auth status >/dev/null 2>&1; then
  echo "Sign in first with: gh auth login" >&2
  exit 2
fi

EXPECTED="Ehsanamn1/alpha"
CURRENT_LOGIN="$(gh api user --jq .login)"
if [ "$CURRENT_LOGIN" != "Ehsanamn1" ]; then
  echo "Authenticated as $CURRENT_LOGIN, expected Ehsanamn1. Stopping to avoid publishing to the wrong account." >&2
  exit 2
fi
gh auth setup-git

if [ ! -d .git ]; then
  git init -b main
fi
if [ "$(git branch --show-current)" != "main" ]; then
  echo "Current branch must be main." >&2
  exit 2
fi
if ! git config user.name >/dev/null 2>&1; then
  git config user.name "$(gh api user --jq '(.name // .login)')"
fi
if ! git config user.email >/dev/null 2>&1; then
  GH_ID="$(gh api user --jq .id)"
  git config user.email "${GH_ID}+Ehsanamn1@users.noreply.github.com"
fi

if ! gh repo view "$EXPECTED" >/dev/null 2>&1; then
  echo "Repository $EXPECTED was not found or is not accessible." >&2
  exit 2
fi
if git remote get-url origin >/dev/null 2>&1; then
  REMOTE="$(git remote get-url origin)"
  case "$REMOTE" in
    *github.com/Ehsanamn1/alpha.git|*github.com:Ehsanamn1/alpha.git) ;;
    *) echo "Existing origin ($REMOTE) is not $EXPECTED; refusing to overwrite it." >&2; exit 2;;
  esac
else
  git remote add origin "https://github.com/$EXPECTED.git"
fi

git add -A
if ! git diff --cached --quiet; then
  git commit -m "Publish Alpha 0.3.0-alpha source"
fi

REMOTE_LINE="$(git ls-remote origin refs/heads/main)"
if [ -n "$REMOTE_LINE" ]; then
  REMOTE_SHA="$(printf '%s' "$REMOTE_LINE" | awk '{print $1}')"
  git fetch origin main
  REMOTE_FILES="$(git ls-tree -r --name-only origin/main)"
  if [ -n "$REMOTE_FILES" ]; then
    echo "Remote main contains files already; refusing to force-push. Review https://github.com/$EXPECTED first." >&2
    exit 2
  fi
  git push "--force-with-lease=refs/heads/main:$REMOTE_SHA" -u origin main
else
  git push -u origin main
fi

TAG="v0.3.0-alpha"
if git ls-remote --exit-code --tags origin "refs/tags/$TAG" >/dev/null 2>&1; then
  echo "Tag $TAG already exists. Source was pushed; refusing to move the existing tag." >&2
  exit 2
fi
git tag "$TAG"
git push origin "$TAG"

echo "Source and release tag pushed. Check Actions and Releases; do not share download links until the workflow succeeds."
echo "https://github.com/$EXPECTED/actions"
echo "https://github.com/$EXPECTED/releases"
