-- Fills the bench database with finished history for the lab tenant: rows
-- messages, one succeeded delivery each, and one 2xx attempt each, spread
-- over the last 30 days. Run it after setup.sh has created the tenant.
--
--   psql "$BENCH_DB_URL" -v rows=5000000 -f seed.sql
\set ON_ERROR_STOP on

SELECT t.id AS tenant_id, e.id AS endpoint_id
FROM tenants t JOIN endpoints e ON e.tenant_id = t.id
WHERE t.name = 'lab-02'
\gset

\timing on
BEGIN;

-- uuidv7(shift) back-dates the ID to match created_at, so IDs keep the same
-- time order they would have had if the rows had arrived over those 30 days.
CREATE TEMP TABLE seed ON COMMIT DROP AS
SELECT uuidv7(-age) AS message_id,
       uuidv7(-age) AS delivery_id,
       now() - age  AS created_at
FROM (
    SELECT make_interval(secs => random() * 30 * 86400) AS age
    FROM generate_series(1, :rows)
) ages;

INSERT INTO messages (id, tenant_id, event_type, payload, created_at)
SELECT message_id, :'tenant_id', 'booking.created',
       jsonb_build_object('seeded', true, 'n', row_number() OVER ()),
       created_at
FROM seed;

INSERT INTO deliveries (id, message_id, endpoint_id, tenant_id, status,
                        attempt_count, next_attempt_at, created_at, updated_at)
SELECT delivery_id, message_id, :'endpoint_id', :'tenant_id', 'succeeded',
       1, created_at, created_at, created_at + interval '60 ms'
FROM seed;

INSERT INTO attempts (delivery_id, tenant_id, status_code, duration_ms, created_at)
SELECT delivery_id, :'tenant_id', 204, 50, created_at + interval '60 ms'
FROM seed;

COMMIT;

VACUUM (ANALYZE) messages, deliveries, attempts;
\timing off

SELECT relname,
       n_live_tup,
       pg_size_pretty(pg_total_relation_size(relid)) AS total_size
FROM pg_stat_user_tables
WHERE relname IN ('messages', 'deliveries', 'attempts')
ORDER BY relname;

SELECT pg_size_pretty(pg_relation_size('deliveries_due_idx')) AS due_index_size;
