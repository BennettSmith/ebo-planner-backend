-- 000001_init.down.sql
--
-- Development-friendly teardown for the init migration.
-- Drops all objects created in the public schema (tables, views, functions, types, etc.)
-- and recreates the public schema with common grants.

DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;

-- Reapply common default grants (Postgres defaults).
GRANT ALL ON SCHEMA public TO postgres;
GRANT ALL ON SCHEMA public TO public;
