-- 0007 — Writing (AI correction), AI conversation, and Busuu-style community feedback.
CREATE TABLE writing_submissions (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  prompt        text,
  body          text NOT NULL,
  cefr_estimate text,
  corrections   jsonb,                          -- [{span,type,suggestion}]
  status        text NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending','scored','community_reviewed','teacher_reviewed')),
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_writing_user ON writing_submissions(user_id, created_at DESC);

CREATE TABLE ai_conversations (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  scenario     text,
  cefr_cap     text NOT NULL DEFAULT 'A1',      -- model must stay within level
  messages     jsonb NOT NULL DEFAULT '[]',
  created_at   timestamptz NOT NULL DEFAULT now()
);

-- Busuu signature: learners submit writing/speaking; NATIVE speakers correct it.
CREATE TABLE community_submissions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind         text NOT NULL CHECK (kind IN ('writing','speaking')),
  language     text NOT NULL,                   -- language being learned
  body         text,
  audio_key    text,
  status       text NOT NULL DEFAULT 'open'
               CHECK (status IN ('open','reviewed','closed')),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_comm_open ON community_submissions(language, created_at DESC)
  WHERE status = 'open';

CREATE TABLE community_feedback (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  submission_id uuid NOT NULL REFERENCES community_submissions(id) ON DELETE CASCADE,
  reviewer_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  corrected_text text,
  comment       text,
  helpful_votes int NOT NULL DEFAULT 0,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_comm_feedback_sub ON community_feedback(submission_id);
