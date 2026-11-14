DO $$ BEGIN
    RAISE EXCEPTION 'course location is retained to protect learner resume history';
END $$;
