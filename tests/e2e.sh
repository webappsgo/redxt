#!/usr/bin/env bash
# @@License : WTFPL
#
# Browser E2E suite, per AI.md PART 29 "tests/e2e.sh". Runs the
# chromedp-based Go tests under tests/e2e (build tag "e2e") inside the
# casjaysdev/go:latest toolchain image, which carries a headless
# Chromium binary. Never part of `make test` — this is developer/CI
# initiated only, since it needs a real browser engine.
set -eo pipefail

PROJECT_NAME=$(basename "$PWD")

GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/${PROJECT_NAME}}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

echo "Running browser E2E suite in Docker (casjaysdev/go:latest + chromium)..."
docker run --rm \
  --name "${PROJECT_NAME}-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
  -v "$PWD:/app" \
  -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
  -v "$GO_BUILD:/usr/local/share/go/cache" \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest sh -c "
    set -e
    if ! command -v chromium >/dev/null 2>&1 && \
       ! command -v chromium-browser >/dev/null 2>&1 && \
       ! command -v google-chrome >/dev/null 2>&1; then
      apk add --no-cache chromium >/dev/null 2>&1 || \
        (apt-get update >/dev/null 2>&1 && apt-get install -y chromium >/dev/null 2>&1) || \
        echo 'WARNING: could not install a headless-Chromium binary; browser tiers will skip themselves'
    fi
    go test -tags e2e -v ./tests/e2e/...
  "

echo "E2E tests completed"
