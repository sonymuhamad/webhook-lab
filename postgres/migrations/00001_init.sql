-- +goose Up
CREATE TABLE tenants (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE api_keys (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id  uuid NOT NULL REFERENCES tenants (id),
    key_hash   bytea NOT NULL UNIQUE,
    prefix     text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz
);

CREATE INDEX api_keys_tenant_id_idx ON api_keys (tenant_id);

CREATE TABLE endpoints (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    url         text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    disabled_at timestamptz
);

CREATE INDEX endpoints_tenant_id_idx ON endpoints (tenant_id) WHERE disabled_at IS NULL;

CREATE TABLE messages (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    tenant_id       uuid NOT NULL REFERENCES tenants (id),
    event_type      text NOT NULL,
    payload         jsonb NOT NULL,
    idempotency_key text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- NULLs are distinct here, so messages sent without a key never conflict.
    UNIQUE (tenant_id, idempotency_key)
);

-- The outbox: one row per message per endpoint.
CREATE TABLE deliveries (
    id              uuid PRIMARY KEY DEFAULT uuidv7(),
    message_id      uuid NOT NULL REFERENCES messages (id),
    endpoint_id     uuid NOT NULL REFERENCES endpoints (id),
    tenant_id       uuid NOT NULL REFERENCES tenants (id),
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending', 'succeeded', 'failed')),
    attempt_count   integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (message_id, endpoint_id)
);

-- Only pending rows are indexed, so the worker's poll stays cheap no matter
-- how many deliveries have already finished.
CREATE INDEX deliveries_due_idx ON deliveries (next_attempt_at) WHERE status = 'pending';

-- Left unpartitioned on purpose: lab 4 measures what it costs to fix that later.
CREATE TABLE attempts (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    delivery_id uuid NOT NULL REFERENCES deliveries (id),
    tenant_id   uuid NOT NULL REFERENCES tenants (id),
    status_code integer,
    error       text,
    duration_ms integer NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX attempts_delivery_id_idx ON attempts (delivery_id);

-- +goose Down
DROP TABLE attempts, deliveries, messages, endpoints, api_keys, tenants;
