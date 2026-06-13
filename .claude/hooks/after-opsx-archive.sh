#!/usr/bin/env bash
# PostToolUse hook (Bash, matcher: mv *).
# Detects the `mv openspec/changes/<name> openspec/changes/archive/...` step
# performed by /opsx:archive, and prompts the model to regenerate the README
# "Specs" section via /dev:update-readme-specs.
set -euo pipefail

cmd=$(jq -r '.tool_input.command // ""')

if [[ "$cmd" == mv\ * ]] && [[ "$cmd" == *"openspec/changes/archive/"* ]]; then
  cat <<'EOF'
{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"The /opsx:archive command just archived a change into openspec/changes/archive/. Per project automation, now follow the steps in .claude/commands/dev/update-readme-specs.md to regenerate the \"Specs\" section in README.md."}}
EOF
fi
