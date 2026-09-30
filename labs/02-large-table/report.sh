#!/usr/bin/env bash
# Waits until the live tenants of this run have no pending delivery, then
# prints what the receiver saw and what the database recorded for them.
# Run it after k6 finishes, with .lab.env sourced (it sets RUN).
#
# Only the tenants named lab-02-$RUN-* count: the bench database also holds
# the seeded history and every earlier run.
set -euo pipefail

db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
stats=${RECEIVER_STATS:-http://localhost:9000/stats}
timeout=${TIMEOUT:-600}
run=${RUN:?RUN is required: source labs/02-large-table/.lab.env}

live="SELECT id FROM tenants WHERE name LIKE 'lab-02-$run-%'"

for ((waited = 0; waited < timeout; waited++)); do
  pending=$(psql "$db" -Atc "SELECT count(*) FROM deliveries WHERE status = 'pending' AND tenant_id IN ($live)")
  [[ $pending == 0 ]] && break
  sleep 1
done
if [[ $pending != 0 ]]; then
  echo "still $pending pending after ${timeout}s; the numbers below are partial" >&2
fi
echo "drained at $(date +%s)"

echo "== receiver"
curl -sS "$stats" | jq .

# attempts has no index on tenant_id, so these read the whole table,
# seeded rows included.
echo "== database (run $run)"
psql "$db" -c "
  WITH d AS (SELECT * FROM deliveries WHERE tenant_id IN ($live)),
       a AS (SELECT * FROM attempts WHERE tenant_id IN ($live))
  SELECT (SELECT count(*) FROM messages WHERE tenant_id IN ($live)) AS messages,
         (SELECT count(*) FROM d)                                   AS deliveries,
         (SELECT count(*) FROM d WHERE status = 'succeeded')        AS succeeded,
         (SELECT count(*) FROM d WHERE status = 'failed')           AS failed,
         (SELECT count(*) FROM a)                                   AS attempts,
         (SELECT count(*) FROM a WHERE status_code NOT BETWEEN 200 AND 299
                                    OR status_code IS NULL)         AS failed_attempts,
         (SELECT count(*) FROM d WHERE attempt_count > 1)           AS retried_deliveries"

# A delivery with more than one 2xx attempt was sent successfully more than once.
echo "== database duplicates"
psql "$db" -c "
  SELECT count(*)                 AS deliveries_sent_twice_or_more,
         coalesce(sum(ok - 1), 0) AS extra_successful_sends
  FROM (SELECT delivery_id, count(*) AS ok
        FROM attempts
        WHERE status_code BETWEEN 200 AND 299 AND tenant_id IN ($live)
        GROUP BY delivery_id) per_delivery
  WHERE ok > 1"
