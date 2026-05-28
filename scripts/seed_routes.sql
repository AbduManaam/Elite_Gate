INSERT INTO upstreams (name, target_url, protocol, health_path) VALUES
  ('http-user-service',  'http://http-user:9001',  'http', '/health'),
  ('http-order-service', 'http://http-order:9002', 'http', '/health'),
  ('grpc-hello-service', 'grpc-hello:50052',         'grpc', NULL)
ON CONFLICT (name) DO NOTHING;

INSERT INTO routes (path, upstream_url, methods, protocol, match_type, enabled)
SELECT '/api/users',  'http://http-user:9001',  ARRAY['GET','POST','PUT','DELETE'], 'http', 'prefix', TRUE
WHERE NOT EXISTS (SELECT 1 FROM routes WHERE path = '/api/users');

INSERT INTO routes (path, upstream_url, methods, protocol, match_type, enabled)
SELECT '/api/orders', 'http://http-order:9002', ARRAY['GET','POST','PUT','DELETE'], 'http', 'prefix', TRUE
WHERE NOT EXISTS (SELECT 1 FROM routes WHERE path = '/api/orders');

INSERT INTO routes (path, upstream_url, methods, protocol, match_type, enabled)
SELECT '/helloworld.v1.Greeter', 'grpc-hello:50052', ARRAY['*'], 'grpc', 'prefix', TRUE
WHERE NOT EXISTS (SELECT 1 FROM routes WHERE path = '/helloworld.v1.Greeter');
