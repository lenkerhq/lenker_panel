CREATE TABLE routing_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id UUID NULL REFERENCES nodes(id) ON DELETE CASCADE,
    rule_type VARCHAR(50) NOT NULL,
    target TEXT NOT NULL,
    action VARCHAR(50) NOT NULL,
    outbound_tag VARCHAR(100) NULL,
    priority INTEGER NOT NULL DEFAULT 100,
    enabled BOOLEAN NOT NULL DEFAULT true,
    description TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_routing_rules_node_id ON routing_rules(node_id);
CREATE INDEX idx_routing_rules_priority ON routing_rules(priority);
