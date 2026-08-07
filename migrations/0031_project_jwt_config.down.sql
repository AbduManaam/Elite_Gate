DROP POLICY IF EXISTS project_jwt_configs_tenant_isolation
    ON project_jwt_configs;

DROP TRIGGER IF EXISTS trg_project_jwt_configs_updated_at
    ON project_jwt_configs;

DROP TABLE IF EXISTS project_jwt_configs;