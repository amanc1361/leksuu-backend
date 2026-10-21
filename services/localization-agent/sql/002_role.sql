-- DBA only. Set a generated password interactively with: \password localization_service
-- Use a fresh role; do not grant application roles or public schema CREATE.
CREATE ROLE localization_service LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT;
GRANT USAGE ON SCHEMA public,localization TO localization_service;
GRANT SELECT ON public.courses,public.units,public.lessons,public.lesson_items,public.exercises TO localization_service;
GRANT SELECT,INSERT,UPDATE ON localization.runs,localization.jobs TO localization_service;
GRANT SELECT,INSERT ON localization.drafts TO localization_service;
GRANT UPDATE(needs_review) ON localization.drafts TO localization_service;
ALTER ROLE localization_service SET statement_timeout = '10s';
ALTER ROLE localization_service SET lock_timeout = '3s';
-- Audit inherited PUBLIC privileges before production use. Service must not be a DB/schema owner.

GRANT EXECUTE ON FUNCTION localization.lock_source() TO localization_service;
