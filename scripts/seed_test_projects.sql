-- Seed script to setup Project Alpha, Project Beta, users, and memberships for multi-tenant Postman tests

-- Ensure pgcrypto extension is active to generate bcrypt hashes dynamically
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- 1. Create or update the 'admin' user (username: admin, password: COreGuard@Admin!) using native pgcrypto crypt()
INSERT INTO admin_users (id, username, password_hash, email, is_active)
VALUES (
  '11111111-1111-1111-1111-111111111111',
  'admin',
  crypt('COreGuard@Admin!', gen_salt('bf', 10)),
  'admin@elitegate.local',
  TRUE
)
ON CONFLICT (username) DO UPDATE SET password_hash = crypt('COreGuard@Admin!', gen_salt('bf', 10));

-- 2. Create or update the 'collaborator' user (username: collaborator, password: COreGuard@Admin!, ID: 22222222-2222-2222-2222-222222222222)
INSERT INTO admin_users (id, username, password_hash, email, is_active)
VALUES (
  '22222222-2222-2222-2222-222222222222',
  'collaborator',
  crypt('COreGuard@Admin!', gen_salt('bf', 10)),
  'collaborator@elitegate.local',
  TRUE
)
ON CONFLICT (username) DO UPDATE SET password_hash = crypt('COreGuard@Admin!', gen_salt('bf', 10));

-- 3. Ensure Default Project exists and is owned by 'admin'
INSERT INTO projects (id, name, slug, owner_id)
VALUES (
  '00000000-0000-0000-0000-000000000000',
  'Default Project',
  'default',
  (SELECT id FROM admin_users WHERE username = 'admin' LIMIT 1)
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO project_members (project_id, admin_user_id, role)
VALUES (
  '00000000-0000-0000-0000-000000000000',
  (SELECT id FROM admin_users WHERE username = 'admin' LIMIT 1),
  'owner'
)
ON CONFLICT (project_id, admin_user_id) DO NOTHING;

-- 4. Insert Project Alpha (aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa) owned by 'admin'
INSERT INTO projects (id, name, slug, owner_id)
VALUES (
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 
  'Project Alpha', 
  'project-alpha', 
  (SELECT id FROM admin_users WHERE username = 'admin' LIMIT 1)
)
ON CONFLICT (id) DO NOTHING;

-- 5. Insert Project Beta (bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb) owned by 'collaborator'
INSERT INTO projects (id, name, slug, owner_id)
VALUES (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 
  'Project Beta', 
  'project-beta', 
  (SELECT id FROM admin_users WHERE username = 'collaborator' LIMIT 1)
)
ON CONFLICT (id) DO NOTHING;

-- 6. Add the 'admin' user as an owner of Project Alpha
INSERT INTO project_members (project_id, admin_user_id, role)
VALUES (
  'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', 
  (SELECT id FROM admin_users WHERE username = 'admin' LIMIT 1), 
  'owner'
)
ON CONFLICT (project_id, admin_user_id) DO NOTHING;

-- 7. Add the 'collaborator' user as an owner of Project Beta
INSERT INTO project_members (project_id, admin_user_id, role)
VALUES (
  'bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb', 
  (SELECT id FROM admin_users WHERE username = 'collaborator' LIMIT 1), 
  'owner'
)
ON CONFLICT (project_id, admin_user_id) DO NOTHING;
