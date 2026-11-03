CREATE TABLE admin_timing_exceptions (
    source_id text NOT NULL,
    paragraph integer NOT NULL CHECK (paragraph > 0 AND paragraph <= 500),
    audio_sha256 char(64) NOT NULL,
    transcript_sha256 char(64) NOT NULL,
    record_sha256 char(64) NOT NULL,
    record jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source_id, paragraph)
);
