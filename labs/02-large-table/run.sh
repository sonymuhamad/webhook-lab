#!/usr/bin/env bash
# Runs one measured lab 02 run against an API, worker, and receiver that are
# already running (see README → Run): creates new live tenants, samples
# Postgres while k6 sends, waits for the drain, and writes everything to
# results/<name>-*.txt.
#
#   labs/02-large-table/run.sh run2-pool16
#
# Two samplers run during the load. Every 15 s: autovacuum state of
# deliveries, pending count, load, and swap. Every 1 s: what the API's active
# backends wait on, which needs application_name=webhook-api in the API's
# POSTGRES_URL.
set -uo pipefail

name=${1:?usage: run.sh <name>}
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
out=$here/results/$name
db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
rate=${RATE:-1000}
duration=${DURATION:-5m}

table_state() {
  while :; do
    printf '%s load=%s swap=%s ' "$(date +%T)" "$(sysctl -n vm.loadavg | awk '{print $2}')" "$(sysctl -n vm.swapusage | awk '{print $6}')"
    psql "$db" -Atc "
      SELECT 'dead=' || n_dead_tup || ' ins_since_vac=' || n_ins_since_vacuum ||
             ' av_count=' || autovacuum_count || ' last_av=' || coalesce(last_autovacuum::time(0)::text, '-') ||
             ' pending=' || (SELECT count(*) FROM deliveries WHERE status = 'pending')
      FROM pg_stat_user_tables WHERE relname = 'deliveries'"
    sleep 15
  done
}

api_waits() {
  while :; do
    # idle_in_tx: the backend holds a transaction open and waits for the API's
    # next statement. A wait_event of CPU means the backend is running, not waiting.
    psql "$db" -Atc "
      WITH api AS (SELECT * FROM pg_stat_activity WHERE application_name = 'webhook-api')
      SELECT to_char(now(), 'HH24:MI:SS') ||
             ' conns=' || (SELECT count(*) FROM api) ||
             ' idle_in_tx=' || (SELECT count(*) FROM api WHERE state = 'idle in transaction') ||
             ' active=' || coalesce((
               SELECT string_agg(w || ':' || n, ' ' ORDER BY n DESC)
               FROM (SELECT coalesce(wait_event_type || '/' || wait_event, 'CPU') AS w, count(*) AS n
                     FROM api WHERE state = 'active' GROUP BY 1) t), '0')"
    sleep 1
  done
}

cd "$repo"
"$here/setup.sh"
source "$here/.lab.env"

table_state > "$out-samples.txt" 2>&1 &
sampler=$!
api_waits > "$out-waits.txt" 2>&1 &
waits=$!

start=$(date +%s)
echo "run $RUN started at $start"
k6 run -e API_KEYS="$API_KEYS" -e RATE="$rate" -e VUS=1000 -e DURATION="$duration" \
  labs/01-worker-contention/load.js > "$out-k6.txt" 2>&1
echo "k6 exited $? at $(date +%s)"
kill $waits

RUN=$RUN TIMEOUT=900 "$here/report.sh" > "$out-report.txt" 2>&1
end=$(grep -o 'drained at [0-9]*' "$out-report.txt" | awk '{print $3}')
sleep 20 # one more metric export, plus a final table sample
kill $sampler
"$here/metrics.sh" "$start" "$end" >> "$out-report.txt" 2>&1
echo "run $RUN drained at $end; results in results/$name-*.txt"
