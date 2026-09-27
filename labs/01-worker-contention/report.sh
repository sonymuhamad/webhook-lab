#!/usr/bin/env bash
# Waits until no delivery is pending, then prints what the receiver saw and
# what the database recorded. Run it after k6 finishes.
set -euo pipefail

db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
stats=${RECEIVER_STATS:-http://localhost:9000/stats}
timeout=${TIMEOUT:-300}

for ((waited = 0; waited < timeout; waited++)); do
  pending=$(psql "$db" -Atc "SELECT count(*) FROM deliveries WHERE status = 'pending'")
  [[ $pending == 0 ]] && break
  sleep 1
done
if [[ $pending != 0 ]]; then
  echo "still $pending pending after ${timeout}s; the numbers below are partial" >&2
fi

echo "== receiver"
curl -sS "$stats" | jq .

echo "== database"
psql "$db" -c "
  SELECT (SELECT count(*) FROM messages)   AS messages,
         (SELECT count(*) FROM deliveries) AS deliveries,
         (SELECT count(*) FROM attempts)   AS attempts,
         (SELECT count(*) FROM deliveries WHERE status = 'succeeded') AS succeeded,
         (SELECT count(*) FROM deliveries WHERE status = 'failed')    AS failed"

# Cross-check from the database side: a delivery with more than one 2xx
# attempt was sent successfully more than once.
echo "== database duplicates"
psql "$db" -c "
  SELECT count(*)                     AS deliveries_sent_twice_or_more,
         coalesce(sum(ok - 1), 0)     AS extra_successful_sends
  FROM (SELECT delivery_id, count(*) AS ok
        FROM attempts
        WHERE status_code BETWEEN 200 AND 299
        GROUP BY delivery_id) per_delivery
  WHERE ok > 1"
