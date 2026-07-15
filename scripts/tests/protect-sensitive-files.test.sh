#!/bin/bash
# Behavioral tests for plugins/core/hooks/protect-sensitive-files.sh.
#
# Framework-free (portable, no bats dependency): each case feeds a hook JSON
# payload on stdin and asserts the exit code — 2 = BLOCK, 0 = ALLOW. Run
# locally with `bash scripts/tests/protect-sensitive-files.test.sh`; wired into
# CI alongside the shell-syntax/shellcheck gates.
set -uo pipefail

HOOK="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/plugins/core/hooks/protect-sensitive-files.sh"

pass=0
fail=0

# assert <expected-exit> <description> <json-payload>
assert() {
  local expected="$1" desc="$2" payload="$3" actual
  printf '%s' "$payload" | bash "$HOOK" >/dev/null 2>&1
  actual=$?
  if [ "$actual" -eq "$expected" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1))
    printf '  ✗ %s (expected exit %s, got %s)\n' "$desc" "$expected" "$actual"
  fi
}

# jp <path> — build a minimal hook payload naming a target file_path.
jp() { printf '{"tool_input":{"file_path":"%s"}}' "$1"; }

# ── BLOCK (exit 2) ────────────────────────────────────────────────
assert 2 ".env file"                 "$(jp '/repo/.env')"
assert 2 ".env.production"           "$(jp '/repo/.env.production')"
assert 2 "package-lock.json"         "$(jp '/repo/package-lock.json')"
assert 2 "file inside .git/"         "$(jp '/repo/.git/config')"
assert 2 "file inside node_modules/" "$(jp '/repo/node_modules/x/index.js')"
assert 2 "terraform state"           "$(jp '/repo/infra/terraform.tfstate')"
assert 2 "populated values"          "$(jp '/repo/secrets.populated.yaml')"
assert 2 "prod values"               "$(jp '/repo/values.prod.yaml')"
assert 2 "generated .rsc"            "$(jp '/repo/output/router.rsc')"

# ── ALLOW (exit 0) ────────────────────────────────────────────────
assert 0 "ordinary source file"      "$(jp '/repo/src/app.ts')"
assert 0 "README.md"                 "$(jp '/repo/README.md')"
assert 0 "no file_path"              '{"tool_input":{}}'
# Segment match, not substring: docker-build/ must NOT trip the build/ rule.
assert 0 "skills/docker-build/ ok"   "$(jp '/repo/skills/docker-build/x.sh')"
# .env* is a basename prefix; environment.ts is not a dotenv file.
assert 0 "environment.ts not dotenv" "$(jp '/repo/environment.ts')"

printf 'protect-sensitive-files: %d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
