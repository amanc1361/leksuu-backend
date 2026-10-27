CREATE TABLE IF NOT EXISTS course_import_sources (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    file_name      text NOT NULL,
    storage_key    text NOT NULL UNIQUE,
    url            text NOT NULL,
    content_type   text NOT NULL DEFAULT 'application/json',
    content_bytes  bigint NOT NULL DEFAULT 0,
    content_sha256 text NOT NULL,
    metadata       jsonb NOT NULL DEFAULT '{}'::jsonb,
    status         text NOT NULL DEFAULT 'archived',
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_course_import_sources_created
    ON course_import_sources (created_at DESC);

CREATE INDEX IF NOT EXISTS idx_course_import_sources_sha256
    ON course_import_sources (content_sha256);
