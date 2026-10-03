#!/usr/bin/env bash
# Waits until this run's tenants have no pending delivery, then prints the
# delivery lag of each tenant group, by phase and over time.
#
#   RUN=... START=<unix time k6 started> labs/03-noisy-neighbour/report.sh
#
# Lag is measured in the database, per delivery: the time from the message's
# created_at to the delivery's last update, which for a succeeded delivery is
# the moment its successful attempt was saved. A message belongs to the phase
# in which it was created; the phase bounds follow load.js's defaults.
set -euo pipefail

db=${BENCH_DB_URL:-postgres://localhost:5432/webhook_lab_bench?sslmode=disable}
stats=${RECEIVER_STATS:-http://localhost:9000/stats}
timeout=${TIMEOUT:-1800}
run=${RUN:?RUN is required: source labs/03-noisy-neighbour/.lab.env}
start=${START:?START is required: the unix time k6 started}

tenants="SELECT id FROM tenants WHERE name LIKE 'lab-03-$run-%'"

for ((waited = 0; waited < timeout; waited++)); do
  pending=$(psql "$db" -Atc "SELECT count(*) FROM deliveries WHERE status = 'pending' AND tenant_id IN ($tenants)")
  [[ $pending == 0 ]] && break
  sleep 1
done
if [[ $pending != 0 ]]; then
  echo "still $pending pending after ${timeout}s; the numbers below are partial" >&2
fi
echo "drained at $(date +%s)"

echo "== receiver"
curl -sS "$stats" | jq .

# deliveries has no index on tenant_id, so the run's rows are read once into a
# temporary table, and every query below reads that.
psql "$db" -v ON_ERROR_STOP=1 <<SQL
\pset footer off
CREATE TEMP TABLE run AS
SELECT substring(t.name from '-(quiet|bulk|slow)(-[0-9]+)?\$') AS grp,
       d.status, d.attempt_count,
       extract(epoch FROM m.created_at) - $start AS sent_at,
       extract(epoch FROM d.updated_at - m.created_at) AS lag
FROM deliveries d
JOIN messages m ON m.id = d.message_id
JOIN tenants t ON t.id = d.tenant_id
WHERE d.tenant_id IN ($tenants);

\echo == deliveries per group
SELECT grp, count(*) AS deliveries,
       count(*) FILTER (WHERE status = 'succeeded') AS succeeded,
       count(*) FILTER (WHERE status = 'failed')    AS failed,
       count(*) FILTER (WHERE attempt_count > 1)    AS retried,
       round(max(sent_at + lag)::numeric - min(sent_at)::numeric, 1) AS first_sent_to_last_done_s
FROM run GROUP BY grp ORDER BY grp;

\echo == lag by group and phase (seconds; phase = when the message was sent)
SELECT grp,
       CASE WHEN sent_at < 60  THEN '1 base    0:00-1:00'
            WHEN sent_at < 80  THEN '2 bulk    1:00-1:20'
            WHEN sent_at < 600 THEN '3 drain   1:20-10:00'
            WHEN sent_at < 720 THEN '4 slow   10:00-12:00'
            ELSE                    '5 after  12:00-end' END AS phase,
       count(*) AS deliveries,
       round(percentile_cont(0.5)  WITHIN GROUP (ORDER BY lag)::numeric, 2) AS p50,
       round(percentile_cont(0.99) WITHIN GROUP (ORDER BY lag)::numeric, 2) AS p99,
       round(max(lag)::numeric, 2) AS max
FROM run WHERE status = 'succeeded'
GROUP BY 1, 2 ORDER BY 1, 2;

\echo == quiet tenants over time (30 s buckets of send time)
SELECT to_char(make_interval(secs => floor(sent_at / 30) * 30), 'MI:SS') AS sent,
       count(*) AS deliveries,
       round(percentile_cont(0.5)  WITHIN GROUP (ORDER BY lag)::numeric, 2) AS p50,
       round(percentile_cont(0.99) WITHIN GROUP (ORDER BY lag)::numeric, 2) AS p99,
       round(max(lag)::numeric, 2) AS max
FROM run WHERE grp = 'quiet' AND status = 'succeeded'
GROUP BY floor(sent_at / 30) ORDER BY floor(sent_at / 30);
SQL

# A delivery with more than one 2xx attempt was sent successfully more than once.
echo "== database duplicates"
psql "$db" -c "
  SELECT count(*) AS deliveries_sent_twice_or_more
  FROM (SELECT delivery_id FROM attempts
        WHERE status_code BETWEEN 200 AND 299 AND tenant_id IN ($tenants)
        GROUP BY delivery_id HAVING count(*) > 1) d"
