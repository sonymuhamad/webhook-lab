-- name: CreateDeliveriesForMessage :execrows
INSERT INTO deliveries (message_id, endpoint_id, tenant_id)
SELECT @message_id::uuid, e.id, e.tenant_id
FROM endpoints e
WHERE e.tenant_id = @tenant_id::uuid
  AND e.disabled_at IS NULL;

-- name: ListDueDeliveries :many
-- The status filter is a literal, not a parameter, so the planner can match
-- it to the partial index deliveries_due_idx.
--
-- No row locks and no lease: workers running this at once read the same rows
-- and all send them. Used only by CLAIM_MODE=naive, for lab 01.
SELECT d.id, d.message_id, d.tenant_id, d.endpoint_id, d.attempt_count,
       e.url AS endpoint_url, m.event_type, m.payload, m.created_at AS message_created_at
FROM deliveries d
JOIN endpoints e ON e.id = d.endpoint_id
JOIN messages m ON m.id = d.message_id
WHERE d.status = 'pending'
  AND d.next_attempt_at <= now()
ORDER BY d.next_attempt_at
LIMIT $1;

-- name: ListDeliveriesByMessage :many
SELECT d.id, d.message_id, d.endpoint_id, e.url AS endpoint_url, d.status,
       d.attempt_count, d.next_attempt_at, d.created_at, d.updated_at
FROM deliveries d
JOIN endpoints e ON e.id = d.endpoint_id
WHERE d.message_id = $1
ORDER BY d.id;

-- name: UpdateDeliveryAfterAttempt :exec
UPDATE deliveries
SET status          = $2,
    attempt_count   = attempt_count + 1,
    next_attempt_at = $3,
    updated_at      = now()
WHERE id = $1;

-- name: CreateAttempt :exec
INSERT INTO attempts (delivery_id, tenant_id, status_code, error, duration_ms)
VALUES ($1, $2, $3, $4, $5);

-- name: ListAttemptsByDeliveries :many
SELECT id, delivery_id, status_code, error, duration_ms, created_at
FROM attempts
WHERE delivery_id = ANY(@delivery_ids::uuid[])
ORDER BY id;

-- name: CountPendingDeliveries :one
SELECT count(*) FILTER (WHERE next_attempt_at <= now()) AS due,
       count(*) FILTER (WHERE next_attempt_at > now())  AS scheduled
FROM deliveries
WHERE status = 'pending';

-- name: ClaimDueDeliveries :many
-- Claims due deliveries by pushing next_attempt_at forward by the lease, so
-- other workers stop seeing them as due until the lease runs out.
--
-- SKIP LOCKED only covers the moment of claiming: concurrent claims pass over
-- rows another claim is taking instead of waiting for it. The row locks end
-- when this statement commits, before anything is sent; from then on the
-- lease is what keeps other workers away.
--
-- MATERIALIZED makes Postgres run the locking select once, as its own step.
-- Without it, Postgres 12+ may inline a CTE referenced once into the UPDATE,
-- and a planner that rescans it could lock more rows than the batch size.
-- Returned rows are in no particular order.
WITH due AS MATERIALIZED (
    SELECT id
    FROM deliveries
    WHERE status = 'pending'
      AND next_attempt_at <= now()
    ORDER BY next_attempt_at
    LIMIT @batch_size
    FOR UPDATE SKIP LOCKED
), claimed AS (
    UPDATE deliveries d
    SET next_attempt_at = now() + make_interval(secs => @lease_seconds::float8),
        updated_at      = now()
    FROM due
    WHERE d.id = due.id
    RETURNING d.id, d.message_id, d.endpoint_id, d.tenant_id, d.attempt_count
)
SELECT c.id, c.message_id, c.tenant_id, c.endpoint_id, c.attempt_count,
       e.url AS endpoint_url, m.event_type, m.payload, m.created_at AS message_created_at
FROM claimed c
JOIN endpoints e ON e.id = c.endpoint_id
JOIN messages m ON m.id = c.message_id;

-- name: ClaimDueDeliveriesFair :many
-- Like ClaimDueDeliveries, but takes the batch from the tenants in turns:
-- every tenant's oldest due delivery first, then every tenant's second, and
-- so on. A tenant with a large backlog then gets one slot per turn instead
-- of the whole batch.
--
-- candidates reads at most @per_tenant rows per tenant through
-- deliveries_due_by_tenant_idx, so its cost grows with the number of tenants,
-- not with the size of the backlog. It takes no locks: the outer select locks
-- the chosen rows and skips any that another claim holds, then fills the
-- batch from the next candidates. @per_tenant must therefore exceed the batch
-- size, or concurrent claims on a single busy tenant come back short.
WITH candidates AS (
    SELECT d.id, d.next_attempt_at, d.turn
    FROM tenants t
    CROSS JOIN LATERAL (
        SELECT id, next_attempt_at, row_number() OVER (ORDER BY next_attempt_at) AS turn
        FROM deliveries
        WHERE tenant_id = t.id
          AND status = 'pending'
          AND next_attempt_at <= now()
        ORDER BY next_attempt_at
        LIMIT @per_tenant
    ) d
), due AS MATERIALIZED (
    SELECT d.id
    FROM deliveries d
    JOIN candidates c ON c.id = d.id
    WHERE d.status = 'pending'
      AND d.next_attempt_at <= now()
    ORDER BY c.turn, c.next_attempt_at
    LIMIT @batch_size
    FOR UPDATE OF d SKIP LOCKED
), claimed AS (
    UPDATE deliveries d
    SET next_attempt_at = now() + make_interval(secs => @lease_seconds::float8),
        updated_at      = now()
    FROM due
    WHERE d.id = due.id
    RETURNING d.id, d.message_id, d.endpoint_id, d.tenant_id, d.attempt_count
)
SELECT c.id, c.message_id, c.tenant_id, c.endpoint_id, c.attempt_count,
       e.url AS endpoint_url, m.event_type, m.payload, m.created_at AS message_created_at
FROM claimed c
JOIN endpoints e ON e.id = c.endpoint_id
JOIN messages m ON m.id = c.message_id;
