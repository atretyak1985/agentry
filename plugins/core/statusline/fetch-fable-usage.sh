#!/usr/bin/env bash
# fetch-fable-usage.sh — fetch the OFFICIAL claude.ai Fable-5 weekly usage window
# and print it as "PCT|RESET" for the statusline's USAGE line.
#
# WHY THIS EXISTS
#   Claude Code pipes only .rate_limits.five_hour / .seven_day into the statusline
#   JSON (verified against CC 2.1.201's embedded schema + a live capture). The Fable
#   window the claude.ai settings→usage page shows ("Fable  27% used  Resets …") comes
#   from a DIFFERENT source: CC's internal endpoint  GET /api/oauth/usage  (the same
#   feed as the in-app `/usage` dialog). There is no on-disk cache of it to reuse, so
#   this helper calls the endpoint itself, exactly the way CC does (rme()/fetchUtilization).
#
# OPT-IN + CREDENTIAL ISOLATION
#   This is the ONLY file in the statusline that touches a credential. It reads Claude
#   Code's own OAuth access token from the macOS Keychain (item "Claude Code-credentials")
#   — your token, your machine. The statusline enables it only when SWARMERY_STATUSLINE_FABLE=1
#   and never blocks on it (it reads a cached file; this helper runs detached in the
#   background, like the weather refresh).
#
# OUTPUT (stdout, non-secret):
#   "<pct>|<reset>"   e.g. "27|1783594740"  or  "27|2026-07-08T12:59:00Z"
#   Empty line on any failure (no token, network error, no Fable window) → the statusline
#   simply omits the FB segment, same graceful degradation as "weather warming up…".
#
# USAGE:
#   fetch-fable-usage.sh            # prints PCT|RESET (or empty)
#   fetch-fable-usage.sh --debug    # prints the raw usage JSON with any token redacted
#
# CAVEAT: /api/oauth/usage is an INTERNAL, undocumented Claude Code endpoint. It may
# change or break on a CC update. This helper is best-effort and fails silent by design.

set -uo pipefail

KEYCHAIN_SERVICE="${SWARMERY_FABLE_KEYCHAIN_SERVICE:-Claude Code-credentials}"
USAGE_URL="${SWARMERY_FABLE_USAGE_URL:-https://api.anthropic.com/api/oauth/usage}"
OAUTH_BETA="${SWARMERY_FABLE_OAUTH_BETA:-oauth-2025-04-20}"
TIMEOUT="${SWARMERY_FABLE_TIMEOUT:-6}"
DEBUG=0
[ "${1:-}" = "--debug" ] && DEBUG=1

fail() { printf '\n'; exit 0; }   # emit empty line, never error the caller

command -v jq   >/dev/null 2>&1 || fail
command -v curl >/dev/null 2>&1 || fail

# ---- 1) read CC's OAuth access token from the Keychain (stays in this process) -------
CRED_JSON="$(security find-generic-password -s "$KEYCHAIN_SERVICE" -w 2>/dev/null)" || fail
[ -n "$CRED_JSON" ] || fail

# Try the known credential shapes; never print the token.
TOKEN="$(printf '%s' "$CRED_JSON" | jq -r '
  .claudeAiOauth.accessToken
  // .claudeAiOauth.access_token
  // .accessToken
  // .access_token
  // .token
  // empty
' 2>/dev/null)"
# If the blob is not JSON, some installs store the raw token string directly.
if [ -z "$TOKEN" ]; then
  case "$CRED_JSON" in
    sk-ant-oat*|sk-ant-*) TOKEN="$CRED_JSON" ;;
  esac
fi
[ -n "$TOKEN" ] || fail

# ---- 2) call the usage endpoint the same way CC does --------------------------------
RESP="$(curl -fsS --max-time "$TIMEOUT" "$USAGE_URL" \
          -H "Authorization: Bearer $TOKEN" \
          -H "Content-Type: application/json" \
          -H "anthropic-beta: $OAUTH_BETA" 2>/dev/null)" || fail
[ -n "$RESP" ] || fail

if [ "$DEBUG" = "1" ]; then
  # Print the usage JSON for troubleshooting, with any token-like string scrubbed as a
  # belt-and-suspenders safety net (the response should not contain secrets).
  printf '%s\n' "$RESP" | sed -E 's/sk-ant-[A-Za-z0-9_-]+/<redacted>/g'
  exit 0
fi

# ---- 3) extract the Fable window → "PCT|RESET" --------------------------------------
# The /api/oauth/usage response is FLAT (windows at top level; NO .rate_limits wrapper).
# The robust, human-labeled source is the .limits[] array, where the Fable window is the
# entry with .scope.model.display_name == "Fable":
#   { "kind":"weekly_scoped", "percent":28, "resets_at":"2026-07-08T09:59:59…+00:00",
#     "scope":{ "model":{ "display_name":"Fable" } } }
# .percent is already an integer 0-100. resets_at is an ISO-8601 UTC string — the
# statusline's fmt_reset_any() parses it. Fallbacks cover API-shape drift.
OUT="$(printf '%s' "$RESP" | jq -r '
  # primary: .limits[] scoped to a model whose display_name matches Fable
  ( [ .limits[]? | select((.scope.model.display_name // "") | test("fable"; "i")) ] | first ) as $f
  # fallback A: any weekly model-scoped limit (Fable is the only scoped weekly window today)
  | ( [ .limits[]? | select(.kind == "weekly_scoped" and (.scope.model != null)) ] | first ) as $g
  # fallback B: legacy per-model bucket shape (utilization 0..1) if a future response uses it
  | ( [ (.rate_limits.model_scoped[]? // .model_scoped[]?) | select(.display_name | test("fable"; "i")) ] | first ) as $m
  | if   $f != null then (($f.percent // 0) | round | tostring) + "|" + (($f.resets_at // "") | tostring)
    elif $g != null then (($g.percent // 0) | round | tostring) + "|" + (($g.resets_at // "") | tostring)
    elif $m != null and ($m.utilization != null)
                    then (($m.utilization * 100) | round | tostring) + "|" + (($m.resets_at // "") | tostring)
    else empty end
' 2>/dev/null)"

[ -n "$OUT" ] || fail
printf '%s\n' "$OUT"
