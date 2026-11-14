DO $$ BEGIN
    RAISE EXCEPTION 'media progress revision is retained to protect concurrent learning progress';
END $$;
