
INSERT INTO policies (name, auth_required, rate_limit_rpm)
SELECT DISTINCT
    CASE
        WHEN auth_required = false THEN 'public'
        WHEN rate_limit_rpm = 0    THEN 'authenticated_unlimited'
        ELSE 'authenticated_' || rate_limit_rpm::TEXT || '_rpm'
    END,
    auth_required,
    rate_limit_rpm
FROM routes
ON CONFLICT (name) DO NOTHING;

UPDATE routes r
SET policy_id = p.id
FROM policies p
WHERE p.auth_required  = r.auth_required
  AND p.rate_limit_rpm = r.rate_limit_rpm;

INSERT INTO route_methods (route_id, method)
SELECT id, m
FROM   routes,
       unnest(methods) AS m
WHERE  methods IS NOT NULL
  AND  array_length(methods, 1) > 0
  AND  m IN ('GET','POST','PUT','DELETE','PATCH','HEAD','OPTIONS')
ON CONFLICT DO NOTHING;
