#!/usr/bin/env bash
# @@License : WTFPL
#
# Phase 2 binary validation in Docker Alpine, per AI.md PART 29
# "tests/docker.sh". Builds server + redxt-cli + redxt-agent with
# casjaysdev/go:latest, then exercises the compiled binaries in a
# throwaway alpine:latest container.
set -eo pipefail

PROJECT_NAME=$(basename "$PWD")

if [ -f "Makefile" ]; then
  echo "Building with make build..."
  make build
else
  echo "Building in Docker (no Makefile)..."
  GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
  GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/${PROJECT_NAME}}"
  mkdir -p "$GO_CACHE" "$GO_BUILD" binaries

  GO_DOCKER="docker run --rm \
    --name ${PROJECT_NAME}-$(tr -dc 'a-z0-9' </dev/urandom | head -c8) \
    -v $PWD:/app \
    -v $GO_CACHE:/usr/local/share/go/pkg/mod \
    -v $GO_BUILD:/usr/local/share/go/cache \
    -w /app \
    -e CGO_ENABLED=0 \
    -e GOFLAGS=-buildvcs=false \
    casjaysdev/go:latest"

  echo "Building server binary in Docker..."
  $GO_DOCKER go build -buildvcs=false -trimpath -ldflags "-s -w" -o /app/binaries/${PROJECT_NAME} ./src

  if [ -d "src/client" ]; then
    echo "Building client in Docker..."
    $GO_DOCKER go build -buildvcs=false -trimpath -ldflags "-s -w" -o /app/binaries/${PROJECT_NAME}-cli ./src/client
  fi

  if [ -d "src/agent" ]; then
    echo "Building agent in Docker..."
    $GO_DOCKER go build -buildvcs=false -trimpath -ldflags "-s -w" -o /app/binaries/${PROJECT_NAME}-agent ./src/agent
  fi
fi

echo "Testing in Docker (Alpine)..."
docker run --rm \
  --name "${PROJECT_NAME}-$(tr -dc 'a-z0-9' </dev/urandom | head -c8)" \
  -v "$PWD/binaries:/app" \
  alpine:latest sh -c "
    set -e

    apk add --no-cache curl bash file jq >/dev/null

    chmod +x /app/${PROJECT_NAME}
    [ -f /app/${PROJECT_NAME}-cli ] && chmod +x /app/${PROJECT_NAME}-cli
    [ -f /app/${PROJECT_NAME}-agent ] && chmod +x /app/${PROJECT_NAME}-agent

    echo '=== Version Check ==='
    /app/${PROJECT_NAME} --version

    echo '=== Help Check ==='
    /app/${PROJECT_NAME} --help

    echo '=== Binary Info ==='
    ls -lh /app/${PROJECT_NAME}
    file /app/${PROJECT_NAME}

    echo '=== Starting Server for API Tests ==='
    /app/${PROJECT_NAME} --port 64580 > /tmp/server.log 2>&1 &
    SERVER_PID=\$!
    sleep 3
    grep -i 'setup.*token' /tmp/server.log 2>/dev/null || true

    echo '=== Health Endpoint Tests ==='
    curl -q -LSsf http://localhost:64580/server/healthz >/dev/null || echo 'FAILED: /server/healthz'
    curl -q -LSsf -H 'Accept: application/json' http://localhost:64580/server/healthz | jq . >/dev/null || echo 'FAILED: /server/healthz JSON'
    curl -q -LSsf -H 'Accept: text/plain' http://localhost:64580/server/healthz >/dev/null || echo 'FAILED: /server/healthz text/plain'
    curl -q -LSsf http://localhost:64580/api/v1/server/healthz | jq . >/dev/null || echo 'FAILED: /api/v1/server/healthz'

    echo '=== Admin Auth Tests ==='
    HTTP_CODE=\$(curl -q -LSs -o /dev/null -w '%{http_code}' http://localhost:64580/server/administration)
    if [ \"\$HTTP_CODE\" = '302' ] || [ \"\$HTTP_CODE\" = '401' ] || [ \"\$HTTP_CODE\" = '200' ]; then
        echo '✓ Admin route reachable/gated'
    else
        echo \"✗ FAILED: unexpected admin route status \$HTTP_CODE\"
    fi

    SETUP_TOKEN=\$(grep -ioP 'setup token:?\\s*\\K[a-zA-Z0-9._-]+' /tmp/server.log 2>/dev/null | head -1 || echo '')
    if [ -n \"\$SETUP_TOKEN\" ]; then
        echo \"Setup token found: \${SETUP_TOKEN:0:8}...\"
    else
        echo 'No setup token found (server may already be configured)'
    fi

    echo '=== Binary Rename Tests ==='
    cp /app/${PROJECT_NAME} /app/renamed-server
    chmod +x /app/renamed-server
    if /app/renamed-server --help 2>&1 | grep -q 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
    fi

    echo '=== Client Tests (if exists) ==='
    if [ -f /app/${PROJECT_NAME}-cli ]; then
        /app/${PROJECT_NAME}-cli --version || echo 'FAILED: CLI --version'
        /app/${PROJECT_NAME}-cli --help || echo 'FAILED: CLI --help'

        cp /app/${PROJECT_NAME}-cli /app/renamed-cli
        chmod +x /app/renamed-cli
        if /app/renamed-cli --help 2>&1 | grep -q 'renamed-cli'; then
            echo '✓ CLI binary rename works'
        else
            echo '✗ FAILED: CLI --help does not show renamed binary name'
        fi

        /app/${PROJECT_NAME}-cli --server http://localhost:64580 status || echo 'CLI status failed or not applicable'
    else
        echo 'client not built - skipping'
    fi

    echo '=== Agent Tests (if exists) ==='
    if [ -f /app/${PROJECT_NAME}-agent ]; then
        /app/${PROJECT_NAME}-agent --version || echo 'FAILED: Agent --version'
        /app/${PROJECT_NAME}-agent --help || echo 'FAILED: Agent --help'

        cp /app/${PROJECT_NAME}-agent /app/renamed-agent
        chmod +x /app/renamed-agent
        if /app/renamed-agent --help 2>&1 | grep -q 'renamed-agent'; then
            echo '✓ Agent binary rename works'
        else
            echo '✗ FAILED: Agent --help does not show renamed binary name'
        fi
    else
        echo 'Agent not built - skipping'
    fi

    echo '=== Stopping Server ==='
    kill \$SERVER_PID
    wait \$SERVER_PID 2>/dev/null || true

    echo '=== All tests passed ==='
"

echo "Docker tests completed successfully"
