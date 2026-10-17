#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f config.json ]; then
  cp config.example.json config.json
  echo "Created config.json. Enable only an independently tested local engine, then run this script again."
  exit 2
fi
if [ ! -x ./dist/alpha-linux-amd64 ]; then
  echo "Build first with: ./scripts/build-release.sh" >&2
  exit 2
fi
exec ./dist/alpha-linux-amd64 ui --config config.json "$@"
