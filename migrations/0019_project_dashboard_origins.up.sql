ALTER TABLE projects
    ADD COLUMN dashboard_allowed_origins TEXT[] NOT NULL DEFAULT '{}';

-- Keep the array bounded to prevent excessive origins
ALTER TABLE projects
    ADD CONSTRAINT dashboard_allowed_origins_max_len
    CHECK (array_length(dashboard_allowed_origins, 1) IS NULL OR array_length(dashboard_allowed_origins, 1) <= 10);
