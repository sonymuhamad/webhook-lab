-- name: CreateMessage :one
INSERT INTO messages (tenant_id, event_type, payload, idempotency_key)
VALUES ($1, $2, $3, $4)
ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
RETURNING id, tenant_id, event_type, payload, idempotency_key, created_at;

-- name: GetMessageByIdempotencyKey :one
SELECT id, tenant_id, event_type, payload, idempotency_key, created_at
FROM messages
WHERE tenant_id = $1
  AND idempotency_key = $2;

-- name: GetMessage :one
SELECT id, tenant_id, event_type, payload, idempotency_key, created_at
FROM messages
WHERE id = $1
  AND tenant_id = $2;
