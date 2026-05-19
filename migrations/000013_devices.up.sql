CREATE TABLE devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    device_fingerprint VARCHAR(255) NOT NULL,
    device_name VARCHAR(255),
    platform VARCHAR(50),
    app_version VARCHAR(50),
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_ip INET,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT devices_subscription_fingerprint_unique UNIQUE (subscription_id, device_fingerprint),
    CONSTRAINT devices_platform_check CHECK (platform IS NULL OR platform IN ('ios', 'android', 'windows', 'macos', 'linux'))
);

CREATE INDEX devices_subscription_id_idx ON devices(subscription_id);
CREATE INDEX devices_last_seen_at_idx ON devices(last_seen_at);
