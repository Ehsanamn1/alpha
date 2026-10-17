Alpha build and test quick reference

Source validation:
  go test -race ./...
  go vet ./...
  node --check internal/localui/web/app.js
  node --check internal/localui/web/sw.js

Build all CLI binaries and checksums:
  ./scripts/build-release.sh

Windows x64 build (on Windows with Go installed):
  build_windows.bat

Public repo bootstrap (requires GitHub CLI and authentication):
  ./scripts/publish-github.sh

This helper can create the public repo in the authenticated Ehsanamn1 account. It does not push a release tag unless explicitly asked via ALPHA_PUBLISH_TAG=1.
