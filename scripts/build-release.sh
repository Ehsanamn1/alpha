#!/usr/bin/env sh
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/alpha-linux-amd64 ./cmd/resilience
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o dist/alpha-linux-arm64 ./cmd/resilience
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/alpha-windows-amd64.exe ./cmd/resilience
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o dist/alpha-darwin-amd64 ./cmd/resilience
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o dist/alpha-darwin-arm64 ./cmd/resilience
(cd dist && sha256sum alpha-* > SHA256SUMS.txt)
echo "Artifacts built in dist/"
