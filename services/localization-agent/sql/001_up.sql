-- APPLY ONLY AFTER OWNER APPROVES. No writes/ALTER to application tables.
BEGIN;
CREATE SCHEMA localization;
CREATE TABLE localization.runs (
 id uuid PRIMARY KEY, status text NOT NULL CHECK(status IN ('running','paused','completed','failed')),
 model text NOT NULL, review_model text NOT NULL, instruction_version text NOT NULL,
 glossary_version text NOT NULL, budget_usd numeric NOT NULL CHECK(budget_usd>0),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE localization.jobs (
 id uuid PRIMARY KEY, run_id uuid NOT NULL REFERENCES localization.runs(id),
 lesson_id uuid NOT NULL, locale text NOT NULL CHECK(locale IN ('fa','ckb')),
 source_hash text NOT NULL, status text NOT NULL DEFAULT 'pending'
 CHECK(status IN ('pending','claimed','draft','failed','stale')),
 claim_token uuid, lease_until timestamptz, attempts int NOT NULL DEFAULT 0,
 error_code text, session_id text, review_session_id text,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(lesson_id,locale,source_hash)
);
CREATE TABLE localization.drafts (
 id uuid PRIMARY KEY, job_id uuid NOT NULL REFERENCES localization.jobs(id),
 source_type text NOT NULL, source_id uuid NOT NULL, field_path text NOT NULL,
 locale text NOT NULL CHECK(locale IN ('fa','ckb')), source_hash text NOT NULL,
 context_hash text NOT NULL, source_text text NOT NULL, translation text NOT NULL,
 supplementary_note text NOT NULL DEFAULT '', issues jsonb NOT NULL DEFAULT '[]',
 review jsonb NOT NULL, model text NOT NULL, review_model text NOT NULL,
 instruction_version text NOT NULL, glossary_version text NOT NULL,
 status text NOT NULL DEFAULT 'needs_human_review' CHECK(status IN ('needs_human_review','human_approved','rejected')),
 needs_review boolean NOT NULL DEFAULT true,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_type,source_id,field_path,locale,context_hash)
);
CREATE INDEX jobs_claim ON localization.jobs(status,lease_until);
-- Briefly prevent source edits/inserts during the final version check and draft commit.
-- SECURITY DEFINER exposes ONLY fixed locks, never SQL or source writes.
CREATE FUNCTION localization.lock_source() RETURNS void LANGUAGE plpgsql
SECURITY DEFINER SET search_path = pg_catalog AS $$
BEGIN
 LOCK TABLE public.courses,public.units,public.lessons,public.lesson_items,public.exercises IN SHARE MODE;
END $$;
REVOKE ALL ON FUNCTION localization.lock_source() FROM PUBLIC;
COMMIT;
