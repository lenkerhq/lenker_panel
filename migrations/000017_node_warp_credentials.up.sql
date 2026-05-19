CREATE TABLE node_warp_credentials (
    node_id UUID PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    private_key TEXT NOT NULL,
    public_key TEXT NOT NULL,
    address TEXT NOT NULL,
    endpoint TEXT NOT NULL DEFAULT 'engage.cloudflareclient.com:2408',
    enabled BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
