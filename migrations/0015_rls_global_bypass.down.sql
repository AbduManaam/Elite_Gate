-- 1. Routes
DROP POLICY IF EXISTS routes_tenant_isolation ON routes;
CREATE POLICY routes_tenant_isolation ON routes
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- 2. Upstreams
DROP POLICY IF EXISTS upstreams_tenant_isolation ON upstreams;
CREATE POLICY upstreams_tenant_isolation ON upstreams
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- 3. Policies
DROP POLICY IF EXISTS policies_tenant_isolation ON policies;
CREATE POLICY policies_tenant_isolation ON policies
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- 4. Gateways
DROP POLICY IF EXISTS gateways_tenant_isolation ON gateways;
CREATE POLICY gateways_tenant_isolation ON gateways
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- 5. Audit Logs
DROP POLICY IF EXISTS audit_logs_tenant_isolation ON audit_logs;
CREATE POLICY audit_logs_tenant_isolation ON audit_logs
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());

-- 6. API Keys
DROP POLICY IF EXISTS api_keys_tenant_isolation ON api_keys;
CREATE POLICY api_keys_tenant_isolation ON api_keys
    USING      (project_id = current_project_id())
    WITH CHECK (project_id = current_project_id());
