#!/usr/bin/env bash
# Hook: PreToolUse (Write|Edit) per agent
# Purpose: Validate teammate only writes to its owned files.
# Exit 0 = allow, Exit 2 = block with message.
#
# Usage: enforce-file-ownership.sh <agent-name>
# Called from each agent's PreToolUse hook.

set -euo pipefail

AGENT="${1:-unknown}"
PROJECT_DIR="${CLAUDE_PROJECT_DIR:-.}"

# Read hook input from stdin
INPUT=$(cat)

# Extract the file path being written
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // .tool_input.path // ""' 2>/dev/null || echo "")

if [ -z "$FILE_PATH" ]; then
  exit 0
fi

# Make path relative to project dir
REL_PATH="${FILE_PATH#$PROJECT_DIR/}"

# Define ownership boundaries per agent
case "$AGENT" in
  architect)
    case "$REL_PATH" in
      docs/architecture/*|docs/iterations/*|*.schema.*|*.openapi.*)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: architect can only write to docs/architecture/, docs/iterations/, and schema/API spec files. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  dev)
    case "$REL_PATH" in
      internal/*|cmd/*|migrations/*|tests/*)
        exit 0
        ;;
      *_test.go)
        exit 0
        ;;
      README.md)
        exit 0
        ;;
      .claude/rules/*)
        exit 0
        ;;
      go.mod|go.sum)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: dev can only write to internal/, cmd/, migrations/, tests/, *_test.go, README.md, .claude/rules/, go.mod, go.sum. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  reviewer)
    case "$REL_PATH" in
      .claude/rules/*)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: reviewer can only write to .claude/rules/. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  local-devops)
    case "$REL_PATH" in
      Dockerfile|docker-compose.*|.dockerignore)
        exit 0
        ;;
      .github/workflows/*)
        exit 0
        ;;
      Makefile|scripts/*)
        exit 0
        ;;
      .env.example|.env.test)
        exit 0
        ;;
      .golangci.yml|.editorconfig|.tool-versions)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: local-devops can only write to Docker files, workflows, Makefile, scripts/, env examples, and tooling configs. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  product-researcher)
    case "$REL_PATH" in
      docs/research/*)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: product-researcher can only write to docs/research/. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  product-strategist)
    case "$REL_PATH" in
      docs/product/*)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: product-strategist can only write to docs/product/. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  product-manager)
    case "$REL_PATH" in
      docs/product/*)
        exit 0
        ;;
      *)
        echo "OWNERSHIP VIOLATION: product-manager can only write to docs/product/. Attempted: $REL_PATH" >&2
        exit 2
        ;;
    esac
    ;;
  *)
    # Unknown agent — allow (lead has no restrictions)
    exit 0
    ;;
esac
