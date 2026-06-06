-- ============================================================
-- Seed script for EliteGate examples backend microservices
-- Compatible with RLS schema v0009
-- ============================================================

-- Clean up any pre-existing sample routes and upstreams to prevent stale DNS name conflicts (e.g. http-user)
DELETE FROM routes WHERE path IN ('/api/users', '/api/orders', '/helloworld.v1.Greeter', '/services.Greeter', '/services.Notification', '/proto.Greeter', '/proto.Notification');
DELETE FROM upstreams WHERE name IN ('http-user-service', 'http-order-service', 'grpc-hello-service');

-- 1. Create policies for the default project (00000000-0000-0000-0000-000000000000)
-- A public policy (no auth) and an authenticated policy (requires JWT)
INSERT INTO policies (id, project_id, name, auth_required, rate_limit_rpm) VALUES
  ('11111111-1111-1111-1111-111111111111', '00000000-0000-0000-0000-000000000000', 'public_policy', FALSE, 0),
  ('22222222-2222-2222-2222-222222222222', '00000000-0000-0000-0000-000000000000', 'auth_policy', TRUE, 100)
ON CONFLICT (project_id, name) DO NOTHING;

-- 2. Insert upstreams pointing to the sample services' container names in 'elitegate_net'
INSERT INTO upstreams (id, project_id, name, target_url, protocol, health_path, enabled) VALUES
  ('33333333-3333-3333-3333-333333333333', '00000000-0000-0000-0000-000000000000', 'http-user-service', 'http://user-service:9001', 'http', '/health', TRUE),
  ('44444444-4444-4444-4444-444444444444', '00000000-0000-0000-0000-000000000000', 'http-order-service', 'http://order-service:9002', 'http', '/health', TRUE),
  ('55555555-5555-5555-5555-555555555555', '00000000-0000-0000-0000-000000000000', 'grpc-hello-service', 'grpc-service:50052', 'grpc', NULL, TRUE)
ON CONFLICT (project_id, name) DO NOTHING;

-- 3. Insert matching routes linking path prefixes to the upstreams and policies
INSERT INTO routes (project_id, name, path, upstream_id, policy_id, match_type, enabled, methods) VALUES
  ('00000000-0000-0000-0000-000000000000', 'user_route', '/api/users', '33333333-3333-3333-3333-333333333333', '11111111-1111-1111-1111-111111111111', 'prefix', TRUE, '{ANY}'::http_method[]),
  ('00000000-0000-0000-0000-000000000000', 'order_route', '/api/orders', '44444444-4444-4444-4444-444444444444', '11111111-1111-1111-1111-111111111111', 'prefix', TRUE, '{ANY}'::http_method[]),
  ('00000000-0000-0000-0000-000000000000', 'grpc_route_greeter', '/services.Greeter', '55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', 'prefix', TRUE, '{ANY}'::http_method[]),
  ('00000000-0000-0000-0000-000000000000', 'grpc_route_notification', '/services.Notification', '55555555-5555-5555-5555-555555555555', '11111111-1111-1111-1111-111111111111', 'prefix', TRUE, '{ANY}'::http_method[])
ON CONFLICT (project_id, name) DO NOTHING;
