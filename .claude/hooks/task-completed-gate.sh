#!/usr/bin/env bash
# Hook: TaskCompleted
# Purpose: Block task completion if build/tests fail.
# Exit 0 = allow, Exit 2 = block with message.
#
# This hook reads the task context from stdin (JSON) and determines
# which checks to run based on the teammate completing the task.

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

# Read hook input from stdin
INPUT=$(cat)

# Extract teammate name (if available)
TEAMMATE=$(echo "$INPUT" | jq -r '.teammate // "unknown"' 2>/dev/null || echo "unknown")

# Determine which checks to run based on teammate
case "$TEAMMATE" in
  implementer)
    echo "Running build check for implementer task..."
    if [ -f "$PROJECT_DIR/Makefile" ]; then
      if ! make -C "$PROJECT_DIR" build 2>&1; then
        echo "BUILD FAILED: Task cannot be completed until the build passes." >&2
        exit 2
      fi
    fi
    ;;
  qa)
    echo "Running test check for qa task..."
    if [ -f "$PROJECT_DIR/Makefile" ]; then
      if ! make -C "$PROJECT_DIR" test 2>&1; then
        echo "TESTS FAILED: Task cannot be completed until all tests pass." >&2
        exit 2
      fi
    fi
    ;;
  *)
    # No gate for other teammates
    ;;
esac

exit 0
