ALTER TABLE gateways
    DROP COLUMN IF EXISTS public_host,
    DROP COLUMN IF EXISTS public_port;
