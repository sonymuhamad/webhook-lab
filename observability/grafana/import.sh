#!/usr/bin/env bash
# Creates the Prometheus and Loki data sources and loads the dashboard into a
# running Grafana. Safe to run again: existing data sources are updated and the
# dashboard is overwritten.
#
#   GRAFANA_URL   default http://localhost:3000
#   GRAFANA_AUTH  default admin:admin (Grafana's first-run login)
set -euo pipefail

url=${GRAFANA_URL:-http://localhost:3000}
auth=${GRAFANA_AUTH:-admin:admin}
dir=$(cd "$(dirname "$0")" && pwd)

api() {
  curl -sS -f -u "$auth" -H 'Content-Type: application/json' "$@"
}

jq -c '.[]' "$dir/datasources.json" | while read -r ds; do
  uid=$(jq -r .uid <<<"$ds")
  if api -o /dev/null "$url/api/datasources/uid/$uid" 2>/dev/null; then
    api -X PUT "$url/api/datasources/uid/$uid" -d "$ds" -o /dev/null
    echo "updated data source $uid"
  else
    api -X POST "$url/api/datasources" -d "$ds" -o /dev/null
    echo "created data source $uid"
  fi
done

jq -c '{dashboard: ., overwrite: true}' "$dir/dashboard.json" \
  | api -X POST "$url/api/dashboards/db" -d @- -o /dev/null
echo "dashboard: $url/d/webhook-lab"
