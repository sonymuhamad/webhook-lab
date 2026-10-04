-- +goose NO TRANSACTION

-- +goose Up
-- The fair claim reads the oldest due deliveries of each tenant in turn. This
-- index answers that with one short range scan per tenant. Like
-- deliveries_due_idx, it holds pending rows only.
--
-- CONCURRENTLY builds it without blocking writes to a large deliveries table,
-- which is why this migration runs outside a transaction.
CREATE INDEX CONCURRENTLY deliveries_due_by_tenant_idx
    ON deliveries (tenant_id, next_attempt_at) WHERE status = 'pending';

-- +goose Down
DROP INDEX CONCURRENTLY deliveries_due_by_tenant_idx;
