#!/usr/bin/env bash
# Hook: TeammateIdle
# Purpose: Redirect idle teammates to pending tasks.
# Exit 0 = allow idle, Exit 2 = send back to work with message.
#
# Checks the shared task list for pending tasks assigned to this teammate.

set -euo pipefail

# Read hook input from stdin
INPUT=$(cat)

TEAMMATE=$(echo "$INPUT" | jq -r '.teammate // "unknown"' 2>/dev/null || echo "unknown")

# Check if there are pending GitHub issues assigned to this teammate's label
if command -v gh &>/dev/null; then
  PENDING=$(gh issue list --label "$TEAMMATE" --label "status:ready" --state open --json number --jq 'length' 2>/dev/null || echo "0")

  if [ "$PENDING" -gt 0 ]; then
    echo "You have $PENDING pending task(s). Please pick up the next one from: gh issue list --label '$TEAMMATE' --label 'status:ready'" >&2
    exit 2
  fi
fi

exit 0
