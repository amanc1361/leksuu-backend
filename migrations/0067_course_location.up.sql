ALTER TABLE user_learning_records DROP CONSTRAINT user_learning_records_kind_check;
ALTER TABLE user_learning_records ADD CONSTRAINT user_learning_records_kind_check
    CHECK (kind IN ('shadow_queue', 'shadow_attempt', 'daily_mix_step', 'course_location'));
