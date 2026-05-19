CREATE TABLE traffic_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    device_id UUID REFERENCES devices(id) ON DELETE SET NULL,
    node_id UUID NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    bytes_up BIGINT NOT NULL DEFAULT 0,
    bytes_down BIGINT NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT traffic_logs_bytes_up_check CHECK (bytes_up >= 0),
    CONSTRAINT traffic_logs_bytes_down_check CHECK (bytes_down >= 0)
);

CREATE INDEX traffic_logs_subscription_recorded_at_idx ON traffic_logs(subscription_id, recorded_at);
CREATE INDEX traffic_logs_device_recorded_at_idx ON traffic_logs(device_id, recorded_at);
CREATE INDEX traffic_logs_node_recorded_at_idx ON traffic_logs(node_id, recorded_at);

CREATE TABLE traffic_quotas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL UNIQUE REFERENCES subscriptions(id) ON DELETE CASCADE,
    bytes_limit BIGINT,
    bytes_used BIGINT NOT NULL DEFAULT 0,
    reset_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT traffic_quotas_bytes_limit_check CHECK (bytes_limit IS NULL OR bytes_limit > 0),
    CONSTRAINT traffic_quotas_bytes_used_check CHECK (bytes_used >= 0)
);

CREATE INDEX traffic_quotas_reset_at_idx ON traffic_quotas(reset_at);
