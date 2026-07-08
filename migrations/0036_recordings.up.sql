CREATE TABLE IF NOT EXISTS recordings (
    id          text PRIMARY KEY,
    class_id    text NOT NULL,
    host_id     text NOT NULL,
    status      text NOT NULL DEFAULT 'recording',
    size_bytes  bigint NOT NULL DEFAULT 0,
    url         text NOT NULL DEFAULT '',
    storage_key text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    ready_at    timestamptz
);
CREATE INDEX IF NOT EXISTS idx_recordings_class ON recordings (class_id, created_at DESC);
