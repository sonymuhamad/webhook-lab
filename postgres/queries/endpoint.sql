-- name: CreateEndpoint :one
INSERT INTO endpoints (tenant_id, url)
VALUES ($1, $2)
RETURNING id, tenant_id, url, created_at;

-- name: ListEndpointsByTenant :many
SELECT id, tenant_id, url, created_at
FROM endpoints
WHERE tenant_id = $1
  AND disabled_at IS NULL
ORDER BY id
LIMIT $2 OFFSET $3;

-- name: CountEndpointsByTenant :one
SELECT count(*)
FROM endpoints
WHERE tenant_id = $1
  AND disabled_at IS NULL;
