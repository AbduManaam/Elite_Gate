-- Migration: 0012_add_super_admin_flag.down.sql
-- Reverses the is_super_admin column addition.

ALTER TABLE admin_users
    DROP COLUMN IF EXISTS is_super_admin;
