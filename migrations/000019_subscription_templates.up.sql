CREATE TABLE subscription_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    description TEXT NULL,
    plan_id UUID NULL REFERENCES plans(id),
    config JSONB NOT NULL DEFAULT '{}',
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL DEFAULT now()
);

INSERT INTO subscription_templates (name, description, is_system, config) VALUES
('trial-7-days', '7 days, 10GB, 1 device', true, '{"duration_days":7,"traffic_limit_bytes":10737418240,"device_limit":1}'),
('monthly-basic', '30 days, 100GB, 3 devices', true, '{"duration_days":30,"traffic_limit_bytes":107374182400,"device_limit":3}'),
('monthly-premium', '30 days, unlimited traffic, 5 devices', true, '{"duration_days":30,"traffic_limit_bytes":null,"device_limit":5}'),
('yearly-basic', '365 days, 1TB, 3 devices', true, '{"duration_days":365,"traffic_limit_bytes":1099511627776,"device_limit":3}');
