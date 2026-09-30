#!/usr/bin/env bash
# Prints the run's numbers from Prometheus. Pass the Unix times the k6 run
# started and the queue finished draining; report.sh prints the latter.
#
#   labs/02-large-table/metrics.sh 1790500000 1790500300
set -euo pipefail

start=$1
end=$2
window=$(( end - start ))
prom=${PROMETHEUS_URL:-http://localhost:9090}

q() {
  curl -sS "$prom/api/v1/query" --data-urlencode "query=$1" --data-urlencode "time=$end" \
    | jq -r '.data.result[0].value[1] // "n/a" | if . == "n/a" then . else (tonumber * 1000 | round / 1000 | tostring) end'
}

echo "window: ${window}s"
echo "claim p50 / p99 (ms):         $(q "1000 * histogram_quantile(0.5, sum by (le) (increase(delivery_claim_duration_seconds_bucket[${window}s])))") / $(q "1000 * histogram_quantile(0.99, sum by (le) (increase(delivery_claim_duration_seconds_bucket[${window}s])))")"
echo "claim avg (ms):               $(q "1000 * sum(increase(delivery_claim_duration_seconds_sum[${window}s])) / sum(increase(delivery_claim_duration_seconds_count[${window}s]))")"
echo "sends/s:                      $(q "sum(increase(delivery_attempts_total[${window}s])) / ${window}")"
echo "failed sends (5xx, no reply): $(q "sum(increase(delivery_attempts_total{outcome!=\"succeeded\"}[${window}s]))")"
echo "pending due peak:             $(q "max_over_time(max(deliveries_pending{state=\"due\"})[${window}s:15s])")"
echo "delivery lag p50 / p99 (s):   $(q "histogram_quantile(0.5, sum by (le) (increase(delivery_lag_seconds_bucket[${window}s])))") / $(q "histogram_quantile(0.99, sum by (le) (increase(delivery_lag_seconds_bucket[${window}s])))")"
echo "api pool acquire waits:       $(q "sum(increase(db_pool_acquire_waits_total{job=\"webhook-api\"}[${window}s]))")"
echo "worker pool acquire waits:    $(q "sum(increase(db_pool_acquire_waits_total{job=\"webhook-worker\"}[${window}s]))")"
