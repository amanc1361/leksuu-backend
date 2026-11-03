-- Shared draft-processing queue for stories, podcasts and films.
-- Results are review artifacts; workers never publish or rewrite lesson_items.
CREATE TABLE IF NOT EXISTS media_processing_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES lesson_items(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('story', 'podcast', 'video')),
    operation text NOT NULL CHECK (operation IN ('synchronize')),
    payload_sha256 text NOT NULL CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
    payload_snapshot jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'review', 'failed', 'cancelled', 'approved')),
    progress integer NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 3),
    cancel_requested boolean NOT NULL DEFAULT false,
    lease_until timestamptz,
    worker_id text,
    result jsonb,
    error_code text,
    events jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS media_processing_jobs_active_unique
    ON media_processing_jobs(item_id, operation, payload_sha256)
    WHERE status IN ('pending', 'running', 'review');
CREATE INDEX IF NOT EXISTS media_processing_jobs_claim
    ON media_processing_jobs(created_at, id) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS media_processing_jobs_item
    ON media_processing_jobs(item_id, created_at DESC);
