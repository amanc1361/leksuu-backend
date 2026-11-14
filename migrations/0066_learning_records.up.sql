CREATE TABLE user_learning_records (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('shadow_queue', 'shadow_attempt', 'daily_mix_step')),
    record_id text NOT NULL CHECK (length(record_id) BETWEEN 1 AND 512),
    value jsonb NOT NULL CHECK (jsonb_typeof(value) = 'object'),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, kind, record_id)
);
