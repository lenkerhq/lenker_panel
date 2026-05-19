CREATE TABLE global_settings (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT NULL,
    updated_by UUID REFERENCES admins(id),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
