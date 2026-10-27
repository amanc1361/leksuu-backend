-- Back up localization schema first. Deletes ALL localization drafts and run history.
BEGIN;
DROP TABLE localization.drafts;
DROP TABLE localization.jobs;
DROP TABLE localization.runs;
DROP FUNCTION localization.lock_source();
DROP SCHEMA localization;
COMMIT;
