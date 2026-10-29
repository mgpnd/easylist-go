#!/bin/bash
set -euo pipefail

# Cloud sessions only: local machines manage their own tools
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

# The OpenSpec skills and /opsx commands call the openspec CLI
OPENSPEC_VERSION=1.15.0
if [ "$(openspec --version 2>/dev/null || true)" != "$OPENSPEC_VERSION" ]; then
  npm install -g "@fission-ai/openspec@$OPENSPEC_VERSION" >&2
fi
