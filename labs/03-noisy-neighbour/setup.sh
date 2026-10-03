#!/usr/bin/env bash
# Creates this run's tenants and writes their API keys to .lab.env:
#
#   QUIET_TENANTS (default 9) quiet tenants, one endpoint each   → /ok/q<i>
#   one bulk tenant with BULK_ENDPOINTS (default 50) endpoints   → /ok/bulk/<j>
#   one slow tenant with one endpoint                            → /slow/s
#
# The receiver tells the groups apart by path, so one receiver stands in for
# every tenant's service. It never deletes: the bench database keeps the data
# of every lab. The API must already be running against BENCH_DB_URL.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)

api=${API:-http://localhost:8080}
receiver=${RECEIVER:-http://localhost:9000}
admin_token=${ADMIN_TOKEN:-$(grep '^ADMIN_TOKEN=' "$repo/.env" | cut -d= -f2-)}
quiet_tenants=${QUIET_TENANTS:-9}
bulk_endpoints=${BULK_ENDPOINTS:-50}

run=$(date +%Y%m%d-%H%M%S)

# create_tenant NAME PATH... prints the new tenant's API key and adds one
# endpoint per path.
create_tenant() {
  local name=$1 key path
  shift
  key=$(curl -sS -f -X POST "$api/admin/tenants" \
    -H "Authorization: Bearer $admin_token" \
    -d "{\"name\":\"lab-03-$run-$name\"}" | jq -r .api_key)
  for path in "$@"; do
    curl -sS -f -o /dev/null -X POST "$api/endpoints" \
      -H "Authorization: Bearer $key" \
      -d "{\"url\":\"$receiver$path\"}"
  done
  echo "$key"
}

quiet_keys=()
for ((i = 1; i <= quiet_tenants; i++)); do
  quiet_keys+=("$(create_tenant "quiet-$i" "/ok/q$i")")
done

bulk_paths=()
for ((j = 1; j <= bulk_endpoints; j++)); do
  bulk_paths+=("/ok/bulk/$j")
done
bulk_key=$(create_tenant bulk "${bulk_paths[@]}")

slow_key=$(create_tenant slow /slow/s)

umask 077
{
  printf 'QUIET_KEYS=%s\n' "$(IFS=,; echo "${quiet_keys[*]}")"
  printf 'BULK_KEY=%s\nSLOW_KEY=%s\nAPI=%s\nRUN=%s\n' "$bulk_key" "$slow_key" "$api" "$run"
} > "$here/.lab.env"
echo "run $run: $quiet_tenants quiet, 1 bulk ($bulk_endpoints endpoints), 1 slow tenant; keys in .lab.env"
