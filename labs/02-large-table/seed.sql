-- Adds 30 days of finished history from tenants that take no live load:
-- :tenants tenants with :endpoints endpoints each and :messages messages per
-- tenant. Every message has one succeeded delivery per endpoint of its tenant,
-- and every delivery has one 2xx attempt.
--
--   psql "$BENCH_DB_URL" -v tenants=100 -v endpoints=50 -v messages=4000 -f seed.sql
--
-- It never deletes: running it again adds another set of history tenants.
\set ON_ERROR_STOP on

SET work_mem = '256MB';
SET maintenance_work_mem = '512MB';

SELECT count(*) + 1 AS first FROM tenants WHERE name LIKE 'history-%' \gset

INSERT INTO tenants (name)
SELECT format('history-%s', lpad(n::text, 4, '0'))
FROM generate_series(:first, :first + :tenants - 1) n;

INSERT INTO endpoints (tenant_id, url)
SELECT t.id, 'http://localhost:9000/hook'
FROM tenants t CROSS JOIN generate_series(1, :endpoints)
WHERE t.name BETWEEN format('history-%s', lpad(:first::text, 4, '0'))
                 AND format('history-%s', lpad((:first + :tenants - 1)::text, 4, '0'));

SELECT set_config('seed.first', :'first', false),
       set_config('seed.last', (:first + :tenants - 1)::text, false),
       set_config('seed.messages', :'messages', false);

-- One transaction per day, oldest day first. uuidv7(shift) back-dates each ID
-- to its created_at, and each insert is sorted by ID, so the primary keys grow
-- at their right edge the way they would have in production.
\timing on
DO $$
DECLARE
    first_tenant text := format('history-%s', lpad(current_setting('seed.first'), 4, '0'));
    last_tenant  text := format('history-%s', lpad(current_setting('seed.last'), 4, '0'));
    per_tenant   int  := current_setting('seed.messages')::int;
BEGIN
    FOR d IN REVERSE 29..0 LOOP
        CREATE TEMP TABLE seed_messages ON COMMIT DROP AS
        SELECT uuidv7(-age) AS id, tenant_id, now() - age AS created_at
        FROM (
            SELECT t.id AS tenant_id, make_interval(secs => (d + random()) * 86400) AS age
            FROM tenants t CROSS JOIN generate_series(1, per_tenant) n
            WHERE t.name BETWEEN first_tenant AND last_tenant AND n % 30 = d
        ) s;

        CREATE TEMP TABLE seed_deliveries ON COMMIT DROP AS
        SELECT uuidv7(m.created_at - now()) AS id, m.id AS message_id,
               e.id AS endpoint_id, m.tenant_id, m.created_at
        FROM seed_messages m JOIN endpoints e ON e.tenant_id = m.tenant_id;

        INSERT INTO messages (id, tenant_id, event_type, payload, created_at)
        SELECT id, tenant_id, 'booking.created', jsonb_build_object('seeded', true), created_at
        FROM seed_messages ORDER BY id;

        INSERT INTO deliveries (id, message_id, endpoint_id, tenant_id, status,
                                attempt_count, next_attempt_at, created_at, updated_at)
        SELECT id, message_id, endpoint_id, tenant_id, 'succeeded',
               1, created_at, created_at, created_at + interval '15 ms'
        FROM seed_deliveries ORDER BY id;

        INSERT INTO attempts (id, delivery_id, tenant_id, status_code, duration_ms, created_at)
        SELECT uuidv7(created_at + interval '15 ms' - now()), id, tenant_id, 204, 10,
               created_at + interval '15 ms'
        FROM seed_deliveries ORDER BY id;

        RAISE NOTICE 'day -% done at %', d, clock_timestamp()::time(0);
        COMMIT;
    END LOOP;
END
$$;

VACUUM (ANALYZE) messages, deliveries, attempts;
\timing off

SELECT relname,
       n_live_tup,
       pg_size_pretty(pg_total_relation_size(relid)) AS total_size
FROM pg_stat_user_tables
WHERE relname IN ('messages', 'deliveries', 'attempts')
ORDER BY relname;

SELECT pg_size_pretty(pg_database_size(current_database())) AS database_size;
