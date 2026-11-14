-- The onboarding rows are account data. An automatic schema rollback would
-- discard completion and answers, causing returning learners to see the wizard.
-- Roll back application code separately. Any later table removal requires an
-- explicit data export, verified restore plan, and a new reviewed migration.
DO $$ BEGIN
  RAISE EXCEPTION '0063 onboarding data cannot be dropped by automatic rollback';
END $$;
