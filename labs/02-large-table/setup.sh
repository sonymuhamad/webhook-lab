#!/usr/bin/env bash
# Resets the bench database and creates tenant lab-02 with one endpoint.
# With ROWS set, also seeds that many finished deliveries (see seed.sql).
# Writes the tenant's API key to .lab.env.
#
#   labs/02-large-table/setup.sh              # run A: empty table
#   ROWS=5000000 labs/02-large-table/setup.sh # run B: 5 million rows of history
#
# The API must already be running against BENCH_DB_URL. Restart the receiver
# before each run too: its counters live in memory.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)

db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
api=${API:-http://localhost:8080}
receiver=${RECEIVER_URL:-http://localhost:9000/hook}
admin_token=${ADMIN_TOKEN:-$(grep '^ADMIN_TOKEN=' "$repo/.env" | cut -d= -f2-)}
rows=${ROWS:-0}

psql "$db" -q -c "TRUNCATE attempts, deliveries, messages, endpoints, api_keys, tenants"
echo "reset $db"

api_key=$(curl -sS -f -X POST "$api/admin/tenants" \
  -H "Authorization: Bearer $admin_token" \
  -d '{"name":"lab-02"}' | jq -r .api_key)

curl -sS -f -o /dev/null -X POST "$api/endpoints" \
  -H "Authorization: Bearer $api_key" \
  -d "{\"url\":\"$receiver\"}"

umask 077
printf 'API_KEY=%s\nAPI=%s\n' "$api_key" "$api" > "$here/.lab.env"
echo "tenant lab-02 → $receiver; API key (${api_key:0:12}…) written to .lab.env"

if (( rows > 0 )); then
  echo "seeding $rows rows of history…"
  psql "$db" -q -v rows="$rows" -f "$here/seed.sql"
fi
