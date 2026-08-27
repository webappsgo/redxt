#!/usr/bin/env bash
# @@License : WTFPL
#
# Auto-detects the available container runtime and runs the matching
# Phase 2 (binary validation) suite, per AI.md PART 29 "Test Scripts".
set -eo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if command -v incus &>/dev/null; then
  echo "Incus detected - running full systemd tests..."
  exec "$SCRIPT_DIR/incus.sh"
elif command -v docker &>/dev/null; then
  echo "Docker detected - running container tests..."
  exec "$SCRIPT_DIR/docker.sh"
else
  echo "ERROR: Neither incus nor docker found"
  echo "Please install one of the following:"
  echo "  - Incus (preferred): https://linuxcontainers.org/incus/"
  echo "  - Docker (fallback): https://docker.com/"
  exit 1
fi
