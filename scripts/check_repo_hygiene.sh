#!/usr/bin/env bash
# scripts/check_repo_hygiene.sh
#
# Reusable repo-hygiene gate. Used both as a local pre-commit check and as a
# CI job (see .github/workflows/ci.yml). Exits non-zero with a clear,
# actionable message on failure so it's easy to debug from a CI log alone.
#
# Usage:
#   ./scripts/check_repo_hygiene.sh            # normal run
#   ./scripts/check_repo_hygiene.sh --verbose  # print every check as it runs

set -euo pipefail

VERBOSE=false
[[ "${1:-}" == "--verbose" ]] && VERBOSE=true

log()  { echo "[hygiene] $*"; }
vlog() { $VERBOSE && echo "[hygiene:debug] $*" || true; }

fail=0

# --- Check 1: any file tracked by git that also matches .gitignore -------
# This is the authoritative check — it only flags files git actually knows
# about, so local build artifacts (bin/*.exe from `go build`) never produce
# a false positive, and force-added ignored files never produce a false
# negative.
vlog "Scanning for git-tracked files that match .gitignore rules..."
tracked_ignored=$(git ls-files -i -c --exclude-standard || true)
if [[ -n "$tracked_ignored" ]]; then
  log "FAIL: the following files are tracked by git but match .gitignore:"
  echo "$tracked_ignored" | sed 's/^/  - /'
  fail=1
else
  log "OK: no gitignored files are tracked."
fi

# --- Check 2: known-bad patterns, even if .gitignore doesn't cover them yet
patterns=("*.exe" "*.dll" "*.log" "*.pyc" "*.class")
for pat in "${patterns[@]}"; do
  matches=$(git ls-files -c -- "$pat" || true)
  if [[ -n "$matches" ]]; then
    log "FAIL: tracked files match banned pattern '$pat':"
    echo "$matches" | sed 's/^/  - /'
    fail=1
  fi
done

# --- Check 3: zero-byte tracked files (placeholders that were never filled in)
vlog "Scanning for zero-byte tracked files..."
empty_files=$(git ls-files -c | while read -r f; do
  [[ -f "$f" && ! -s "$f" ]] && echo "$f"
done || true)
if [[ -n "$empty_files" ]]; then
  log "WARN: zero-byte tracked files found (implement or delete):"
  echo "$empty_files" | sed 's/^/  - /'
  # Warn only, don't fail the build — empty files are a hygiene smell,
  # not a security/size risk, so this shouldn't block merges by itself.
fi

if [[ $fail -ne 0 ]]; then
  log "Repository hygiene check FAILED. See above."
  exit 1
fi

log "Repository hygiene check PASSED."
