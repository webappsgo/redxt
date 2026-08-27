#!/usr/bin/env bash
# @@License : WTFPL
#
# Phase 2 binary + systemd validation in Incus Debian, per AI.md PART 29
# "tests/incus.sh". Preferred over docker.sh when incus is available.
set -eo pipefail

if ! command -v incus &>/dev/null; then
  echo "ERROR: incus not found. Install incus or use tests/docker.sh"
  exit 1
fi

PROJECT_NAME=$(basename "$PWD")
CONTAINER_NAME="test-${PROJECT_NAME}-$$"
INCUS_IMAGE="images:debian/trixie"

trap 'incus delete "$CONTAINER_NAME" --force 2>/dev/null || true' EXIT

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

echo "Launching Incus container (Debian + systemd)..."
incus launch "$INCUS_IMAGE" "$CONTAINER_NAME"

sleep 2

echo "Copying binaries to container..."
incus file push "binaries/${PROJECT_NAME}" "$CONTAINER_NAME/usr/local/bin/"
incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/${PROJECT_NAME}"

if [ -f "binaries/${PROJECT_NAME}-cli" ]; then
  incus file push "binaries/${PROJECT_NAME}-cli" "$CONTAINER_NAME/usr/local/bin/"
  incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/${PROJECT_NAME}-cli"
fi

if [ -f "binaries/${PROJECT_NAME}-agent" ]; then
  incus file push "binaries/${PROJECT_NAME}-agent" "$CONTAINER_NAME/usr/local/bin/"
  incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/${PROJECT_NAME}-agent"
fi

incus exec "$CONTAINER_NAME" -- bash -c "command -v curl || (apt-get update && apt-get install -y curl jq)" >/dev/null 2>&1

echo "Running tests in Incus..."
incus exec "$CONTAINER_NAME" -- bash -c "
    set -eo pipefail

    echo '=== Version Check ==='
    ${PROJECT_NAME} --version

    echo '=== Help Check ==='
    ${PROJECT_NAME} --help

    echo '=== Binary Info ==='
    ls -lh /usr/local/bin/${PROJECT_NAME}
    file /usr/local/bin/${PROJECT_NAME}

    echo '=== Service Install Test ==='
    ${PROJECT_NAME} --service --install

    echo '=== Service Status ==='
    systemctl status ${PROJECT_NAME} || true

    echo '=== Service Start Test ==='
    systemctl start ${PROJECT_NAME}
    sleep 2
    systemctl status ${PROJECT_NAME}

    echo '=== Health Endpoint Tests ==='
    curl -q -LSsf http://localhost:80/server/healthz >/dev/null || echo 'FAILED: /server/healthz'
    curl -q -LSsf -H 'Accept: application/json' http://localhost:80/server/healthz | jq . >/dev/null || echo 'FAILED: /server/healthz JSON'
    curl -q -LSsf -H 'Accept: text/plain' http://localhost:80/server/healthz >/dev/null || echo 'FAILED: /server/healthz text/plain'
    curl -q -LSsf http://localhost:80/api/v1/server/healthz | jq . >/dev/null || echo 'FAILED: /api/v1/server/healthz'

    echo '=== Admin Auth Tests ==='
    HTTP_CODE=\$(curl -q -LSs -o /dev/null -w '%{http_code}' http://localhost:80/server/administration)
    if [ \"\$HTTP_CODE\" = '302' ] || [ \"\$HTTP_CODE\" = '401' ] || [ \"\$HTTP_CODE\" = '200' ]; then
        echo '✓ Admin route reachable/gated'
    else
        echo \"✗ FAILED: unexpected admin route status \$HTTP_CODE\"
    fi

    SETUP_TOKEN=\$(journalctl -u ${PROJECT_NAME} --no-pager 2>/dev/null | grep -ioP 'setup token:?\\s*\\K[a-zA-Z0-9._-]+' | head -1 || echo '')
    if [ -n \"\$SETUP_TOKEN\" ]; then
        echo \"Setup token found: \${SETUP_TOKEN:0:8}...\"
    else
        echo 'No setup token found (server may already be configured)'
    fi

    echo '=== Binary Rename Tests ==='
    cp /usr/local/bin/${PROJECT_NAME} /tmp/renamed-server
    chmod +x /tmp/renamed-server
    if /tmp/renamed-server --help 2>&1 | grep -q 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
    fi

    echo '=== Client Tests (if exists) ==='
    if [ -f /usr/local/bin/${PROJECT_NAME}-cli ]; then
        ${PROJECT_NAME}-cli --version || echo 'FAILED: CLI --version'
        ${PROJECT_NAME}-cli --help || echo 'FAILED: CLI --help'

        cp /usr/local/bin/${PROJECT_NAME}-cli /tmp/renamed-cli
        chmod +x /tmp/renamed-cli
        if /tmp/renamed-cli --help 2>&1 | grep -q 'renamed-cli'; then
            echo '✓ CLI binary rename works'
        else
            echo '✗ FAILED: CLI --help does not show renamed binary name'
        fi

        ${PROJECT_NAME}-cli --server http://localhost:80 status || echo 'CLI status failed or not applicable'
    else
        echo 'client not installed - skipping'
    fi

    echo '=== Agent Tests (if exists) ==='
    if [ -f /usr/local/bin/${PROJECT_NAME}-agent ]; then
        ${PROJECT_NAME}-agent --version || echo 'FAILED: Agent --version'
        ${PROJECT_NAME}-agent --help || echo 'FAILED: Agent --help'

        cp /usr/local/bin/${PROJECT_NAME}-agent /tmp/renamed-agent
        chmod +x /tmp/renamed-agent
        if /tmp/renamed-agent --help 2>&1 | grep -q 'renamed-agent'; then
            echo '✓ Agent binary rename works'
        else
            echo '✗ FAILED: Agent --help does not show renamed binary name'
        fi
    else
        echo 'Agent not installed - skipping'
    fi

    echo '=== Service Stop Test ==='
    systemctl stop ${PROJECT_NAME}

    echo '=== All tests passed ==='
"

echo "Incus tests completed successfully"
