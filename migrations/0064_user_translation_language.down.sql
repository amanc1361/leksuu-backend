-- Removing this column would erase account preferences. Application rollback
-- can keep the additive column; dropping it needs a separate reviewed plan.
DO $$ BEGIN
  RAISE EXCEPTION '0064 translation preferences cannot be dropped automatically';
END $$;
