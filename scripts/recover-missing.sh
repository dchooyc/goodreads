#!/usr/bin/env bash
# Phased, gentle recovery of missing books. Resumable: rerunning skips
# targets already checkpointed in recovery-state.json.
#
# Usage: ./scripts/recover-missing.sh [P0|P1|P2|P3|all] [limit]
set -euo pipefail
cd "$(dirname "$0")/.."

PRIORITY="${1:-P0}"
LIMIT="${2:-0}"

go run ./cmd/recover-missing \
  -targets topreads-missing-books-to-double-check.json \
  -out recovered-missing-books.json \
  -report recovery-report.md \
  -state recovery-state.json \
  -priority "$PRIORITY" \
  -limit "$LIMIT" \
  -workers 1 \
  -delay 3s \
  -timeout 30s \
  -max-retries 3 \
  -keep-previous-on-fail

echo
echo "Inspect recovery-report.md before running the next priority tier."
