#!/usr/bin/env bash
# Smoke: poche sidecar + seed + BFF auth + list proxy.
set -euo pipefail
cd "$(dirname "$0")"
./build.sh

POCHE_BIN="${POCHE_BIN:-$HOME/ai/poche/poche}"
if [[ ! -x "$POCHE_BIN" ]]; then
  (cd "$HOME/ai/poche" && ./build.sh)
  POCHE_BIN="$HOME/ai/poche/poche"
fi

DB=$(mktemp -d /tmp/poche-resend-webmail-XXXX)
export POCHE_DB="$DB"
export POCHE_URL="http://127.0.0.1:17782"
export POCHE_NO_NUDGE=1
export FEEDBACK_RELAY=off
export WEBMAIL_TOKEN="smoke-token-deadbeef"

INIT=$("$POCHE_BIN" init)
export POCHE_TOKEN=$(printf '%s' "$INIT" | sed -n 's/.*"admin_token":"\([^"]*\)".*/\1/p')
test -n "$POCHE_TOKEN"

"$POCHE_BIN" serve 17782 >/tmp/poche-resend-poche.log 2>&1 &
PPID_POCHE=$!
UI_PID=""
cleanup() { kill $PPID_POCHE ${UI_PID:-} 2>/dev/null || true; rm -rf "$DB"; }
trap cleanup EXIT
sleep 0.5

./poche-resend-webmail seed -count 40 | grep -q '"ok":true'
./poche-resend-webmail serve -port 3091 >/tmp/poche-resend-ui.log 2>&1 &
UI_PID=$!
sleep 0.4

curl -sf http://127.0.0.1:3091/api/health | grep -q '"ok":true'
curl -sf http://127.0.0.1:3091/api/config | grep -q '"auth"'
# no token → 401
code=$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:3091/api/status)
test "$code" = "401"
curl -sf -H "Authorization: Bearer $WEBMAIL_TOKEN" http://127.0.0.1:3091/api/status | grep -q 'poche_ok'
curl -sf -H "Authorization: Bearer $WEBMAIL_TOKEN" \
  "http://127.0.0.1:3091/api/messages?missing_link=message_tags.message_id:tag=archive&limit=5" | grep -q '"items"'
curl -sf -H "Authorization: Bearer $WEBMAIL_TOKEN" http://127.0.0.1:3091/api/tags | grep -q '"items"'
# webhook without secret accepts
curl -sf -X POST http://127.0.0.1:3091/webhooks/resend \
  -H 'Content-Type: application/json' \
  -d '{"type":"email.received","data":{"email_id":"nonexistent-smoke"}}' | grep -q '"ok"'
curl -sf "http://127.0.0.1:3091/?token=$WEBMAIL_TOKEN" | grep -q 'poche resend webmail'
./poche-resend-webmail guide | grep -q 'poche-resend-webmail'

# compose: auth required, and validation rejects an empty recipient
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:3091/api/compose -d '{}')
test "$code" = "401"
curl -s -X POST -H "Authorization: Bearer $WEBMAIL_TOKEN" \
  http://127.0.0.1:3091/api/compose -d '{"subject":"x","text":"y"}' | grep -q 'recipient'
# sendable addresses endpoint answers for admin
curl -sf -H "Authorization: Bearer $WEBMAIL_TOKEN" http://127.0.0.1:3091/api/mailbox/addresses | grep -q 'addresses'
# per-mailbox credentials never surface in mailbox list
./poche-resend-webmail mailbox create --address smoke@intrane.fr --password pw \
  --recovery-email r@intrane.fr >/dev/null
SMOKE_KEY=re_smoke_secret_value ./poche-resend-webmail mailbox update \
  --address smoke@intrane.fr --resend-key env:SMOKE_KEY | grep -q '"has_resend_key":true'
./poche-resend-webmail mailbox list | grep -q 're_smoke_secret_value' && { echo "FAIL: key leaked"; exit 1; }
./poche-resend-webmail mailbox list | grep -q '"has_resend_key":true'

echo "OK poche-resend-webmail smoke"
