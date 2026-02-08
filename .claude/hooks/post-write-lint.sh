#!/usr/bin/env bash
# Hook: PostToolUse (Write|Edit)
# Purpose: Async lint after file edits. Non-blocking, feeds warnings back.
#
# Runs the project linter on the modified file. Since this is async,
# it won't block the agent but will surface warnings.

set -euo pipefail

PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

# Read hook input from stdin
INPUT=$(cat)

# Extract the file path that was written/edited
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // ""' 2>/dev/null || echo "")

if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Only lint source files (skip configs, docs, etc.)
case "$FILE_PATH" in
  *.ts|*.tsx|*.js|*.jsx|*.py|*.go|*.rs)
    ;;
  *)
    exit 0
    ;;
esac

# Try available linters in order of preference
if [ -f "$PROJECT_DIR/node_modules/.bin/biome" ]; then
  "$PROJECT_DIR/node_modules/.bin/biome" check "$FILE_PATH" 2>&1 || true
elif [ -f "$PROJECT_DIR/node_modules/.bin/eslint" ]; then
  "$PROJECT_DIR/node_modules/.bin/eslint" "$FILE_PATH" 2>&1 || true
elif command -v ruff &>/dev/null && [[ "$FILE_PATH" == *.py ]]; then
  ruff check "$FILE_PATH" 2>&1 || true
fi

exit 0
