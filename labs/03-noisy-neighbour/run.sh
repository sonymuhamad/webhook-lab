#!/usr/bin/env bash
# Runs one measured lab 03 run against an API, worker, and receiver that are
# already running (see README → Run), and writes results/<name>-*.txt.
#
#   labs/03-noisy-neighbour/run.sh run1-fifo
#
# A sampler records every 15 s while k6 runs and the queue drains: load,
# swap, pending deliveries, the size of the due index, and autovacuum on
# deliveries.
set -uo pipefail

name=${1:?usage: run.sh <name>}
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=$here/results/$name
db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
prom=${PROMETHEUS_URL:-http://localhost:9090}

sample() {
  while :; do
    printf '%s load=%s swap=%s ' "$(date +%T)" "$(sysctl -n vm.loadavg | awk '{print $2}')" "$(sysctl -n vm.swapusage | awk '{print $6}')"
    psql "$db" -Atc "
      SELECT 'pending=' || (SELECT count(*) FROM deliveries WHERE status = 'pending') ||
             ' due_idx=' || pg_size_pretty(pg_relation_size('deliveries_due_idx')) ||
             ' dead=' || n_dead_tup || ' av_count=' || autovacuum_count ||
             ' last_av=' || coalesce(last_autovacuum::time(0)::text, '-')
      FROM pg_stat_user_tables WHERE relname = 'deliveries'"
    sleep 15
  done
}

mkdir -p "$here/results"
cd "$repo"
"$here/setup.sh"
source "$here/.lab.env"

sample > "$out-samples.txt" 2>&1 &
sampler=$!

start=$(date +%s)
echo "run $RUN started at $start"
k6 run -e QUIET_KEYS="$QUIET_KEYS" -e BULK_KEY="$BULK_KEY" -e SLOW_KEY="$SLOW_KEY" \
  "$here/load.js" > "$out-k6.txt" 2>&1
echo "k6 exited $? at $(date +%s)"

RUN=$RUN START=$start "$here/report.sh" > "$out-report.txt" 2>&1
end=$(grep -o 'drained at [0-9]*' "$out-report.txt" | awk '{print $3}')
sleep 20 # one more metric export, plus a final sample
kill $sampler

{
  echo "== prometheus, $start to $end"
  "$repo/labs/02-large-table/metrics.sh" "$start" "$end"
  peak=$(curl -sS "$prom/api/v1/query" \
    --data-urlencode "query=max_over_time(sum(rate(delivery_attempts_total[30s]))[$((end - start))s:15s])" \
    --data-urlencode "time=$end" | jq -r '.data.result[0].value[1] // "n/a"')
  echo "peak sends/s (30 s rate):     $peak"
} >> "$out-report.txt" 2>&1
echo "run $RUN drained at $end; results in results/$name-*.txt"
