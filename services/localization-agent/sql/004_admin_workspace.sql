-- Separate durable admin workspace; existing localization pilot records are untouched.
BEGIN;
CREATE SCHEMA IF NOT EXISTS localization_admin;
CREATE TABLE IF NOT EXISTS localization_admin.segments (
 id uuid PRIMARY KEY,
 lesson_id uuid NOT NULL REFERENCES public.lessons(id) ON DELETE CASCADE,
 field_key text NOT NULL, source_hash text NOT NULL, source_text text NOT NULL,
 locale text NOT NULL CHECK(locale IN ('fa','ckb')),
 translation text NOT NULL DEFAULT '', origin text NOT NULL DEFAULT 'machine',
 status text NOT NULL DEFAULT 'missing', issues jsonb NOT NULL DEFAULT '[]',
 legacy_imported boolean NOT NULL DEFAULT false,
 provider_id text, model text, error_code text, revision integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(lesson_id,field_key,source_hash,locale)
);
ALTER TABLE localization_admin.segments ADD COLUMN IF NOT EXISTS legacy_imported boolean NOT NULL DEFAULT false;
CREATE TABLE IF NOT EXISTS localization_admin.runs (
 id uuid PRIMARY KEY, provider_id text NOT NULL, model text NOT NULL,
 locale text NOT NULL CHECK(locale IN ('fa','ckb')),
 status text NOT NULL DEFAULT 'running', budget_usd numeric NOT NULL CHECK(budget_usd>0),
 reserved_usd numeric NOT NULL DEFAULT 0, estimated_usd numeric NOT NULL DEFAULT 0,
 provider_snapshot jsonb NOT NULL, scope jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS localization_admin.tasks (
 id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES localization_admin.runs(id),
 segment_id uuid NOT NULL REFERENCES localization_admin.segments(id) ON DELETE CASCADE,
 expected_revision integer NOT NULL, status text NOT NULL DEFAULT 'pending',
 phase text NOT NULL DEFAULT 'translate', candidate jsonb, review jsonb,
 error_code text, usage jsonb NOT NULL DEFAULT '[]',
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS admin_segment_active ON localization_admin.tasks(segment_id)
 WHERE status IN ('pending','running');
CREATE INDEX IF NOT EXISTS admin_tasks_run ON localization_admin.tasks(run_id);
CREATE INDEX IF NOT EXISTS admin_tasks_queue ON localization_admin.tasks(status,created_at);
CREATE TABLE IF NOT EXISTS localization_admin.audit (
 id bigserial PRIMARY KEY, actor text NOT NULL, action text NOT NULL,
 segment_id uuid, detail jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
-- Carry forward existing translation-agent drafts without changing the source
-- pilot tables. Published values are detected from live content by the API.
INSERT INTO localization_admin.segments(id,lesson_id,field_key,source_hash,source_text,locale,translation,origin,status,issues,model,revision)
SELECT d.id,j.lesson_id,d.source_type||':'||d.source_id||':'||regexp_replace(d.field_path,'\.'||c.language||'$',''),
 d.source_hash,d.source_text,d.locale,d.translation,
 CASE WHEN d.status='human_approved' THEN 'human' ELSE 'machine' END,
 'draft',d.issues,d.model,1
FROM localization.drafts d JOIN localization.jobs j ON j.id=d.job_id
JOIN public.lessons l ON l.id=j.lesson_id JOIN public.courses c ON c.id=l.course_id
WHERE d.status<>'rejected'
ON CONFLICT DO NOTHING;
UPDATE localization_admin.segments SET legacy_imported=true
WHERE id IN (SELECT id FROM localization.drafts);
COMMIT;
