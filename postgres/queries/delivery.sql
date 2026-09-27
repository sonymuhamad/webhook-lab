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
-- No row locks: two workers running this at once claim the same rows and
-- send them twice. Lab 1 measures that before switching to SKIP LOCKED.
SELECT d.id, d.message_id, d.tenant_id, d.attempt_count,
       e.url AS endpoint_url, m.event_type, m.payload
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
