-- name: CreateTenant :one
INSERT INTO tenants (name)
VALUES ($1)
RETURNING id, name, created_at;

-- name: CreateAPIKey :exec
INSERT INTO api_keys (tenant_id, key_hash, prefix)
VALUES ($1, $2, $3);

-- name: GetTenantByAPIKeyHash :one
SELECT t.id, t.name, t.created_at
FROM tenants t
JOIN api_keys k ON k.tenant_id = t.id
WHERE k.key_hash = $1
  AND k.revoked_at IS NULL;
