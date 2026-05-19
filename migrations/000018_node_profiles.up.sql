CREATE TABLE node_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    description TEXT NULL,
    is_system BOOLEAN NOT NULL DEFAULT false,
    config JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMP NOT NULL DEFAULT now(),
    updated_at TIMESTAMP NOT NULL DEFAULT now()
);

INSERT INTO node_profiles (name, description, is_system, config) VALUES
('default-vless-reality', 'Basic VLESS Reality without routing rules', true, '{"routing_rules":[]}'),
('bypass-china', 'Block geosite:cn and direct geoip:cn', true, '{"routing_rules":[{"rule_type":"geosite","target":"cn","action":"block","priority":10},{"rule_type":"geoip","target":"cn","action":"direct","priority":20}]}'),
('block-ads', 'Block ad domains via geosite:category-ads', true, '{"routing_rules":[{"rule_type":"geosite","target":"category-ads","action":"block","priority":10}]}'),
('with-warp', 'WARP outbound for selected sites', true, '{"routing_rules":[{"rule_type":"geosite","target":"openai","action":"warp","priority":10},{"rule_type":"geosite","target":"netflix","action":"warp","priority":20}]}');
