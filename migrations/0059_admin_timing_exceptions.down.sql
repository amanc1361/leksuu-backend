-- A review queue may contain human evidence. Refuse destructive rollback.
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM admin_timing_exceptions) THEN
        RAISE EXCEPTION 'admin_timing_exceptions is not empty; export and explicitly clear it before rollback';
    END IF;
END $$;
DROP TABLE admin_timing_exceptions;
