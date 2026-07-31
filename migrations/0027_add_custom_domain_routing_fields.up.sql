ALTER TABLE custom_domains
    ADD COLUMN routing_target VARCHAR(253) NOT NULL DEFAULT 'gateway.elitegateway.site',
    ADD COLUMN routing_status VARCHAR(40) NOT NULL DEFAULT 'pending'
        CHECK (routing_status IN ('pending', 'ready', 'failed')),
    ADD COLUMN routing_checked_at TIMESTAMPTZ,
    ADD COLUMN routing_error TEXT;
