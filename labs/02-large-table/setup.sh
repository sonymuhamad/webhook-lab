#!/usr/bin/env bash
# Creates the live tenants that load.js sends as: LIVE_TENANTS tenants
# (default 10) with one endpoint each. Writes their API keys to .lab.env.
# With HISTORY_MESSAGES set, first adds finished history from separate
# tenants (see seed.sql).
#
#   labs/02-large-table/setup.sh                        # live tenants only
#   HISTORY_MESSAGES=4000 labs/02-large-table/setup.sh  # + 100 × 50 × 4,000 = 20 M deliveries
#
# It never deletes: the bench database keeps the data of every lab. The API
# must already be running against BENCH_DB_URL. Restart the receiver before
# each run too: its counters live in memory.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)

db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
api=${API:-http://localhost:8080}
receiver=${RECEIVER_URL:-http://localhost:9000/hook}
admin_token=${ADMIN_TOKEN:-$(grep '^ADMIN_TOKEN=' "$repo/.env" | cut -d= -f2-)}
live_tenants=${LIVE_TENANTS:-10}
history_messages=${HISTORY_MESSAGES:-0}

if (( history_messages > 0 )); then
  echo "seeding history: ${HISTORY_TENANTS:-100} tenants × ${HISTORY_ENDPOINTS:-50} endpoints × $history_messages messages…"
  psql "$db" -q -v tenants="${HISTORY_TENANTS:-100}" -v endpoints="${HISTORY_ENDPOINTS:-50}" \
    -v messages="$history_messages" -f "$here/seed.sql"
fi

# Tenant names must be unique, so each setup run gets its own suffix.
run=$(date +%Y%m%d-%H%M%S)
keys=()
for ((i = 1; i <= live_tenants; i++)); do
  key=$(curl -sS -f -X POST "$api/admin/tenants" \
    -H "Authorization: Bearer $admin_token" \
    -d "{\"name\":\"lab-02-$run-$i\"}" | jq -r .api_key)
  curl -sS -f -o /dev/null -X POST "$api/endpoints" \
    -H "Authorization: Bearer $key" \
    -d "{\"url\":\"$receiver\"}"
  keys+=("$key")
done

umask 077
printf 'API_KEYS=%s\nAPI=%s\nRUN=%s\n' "$(IFS=,; echo "${keys[*]}")" "$api" "$run" > "$here/.lab.env"
echo "$live_tenants live tenants (lab-02-$run-*) → $receiver; API keys written to .lab.env"
