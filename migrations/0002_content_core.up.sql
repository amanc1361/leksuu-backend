-- 0002 — courses, lessons, lesson_items, exercises (order ALWAYS from position)
CREATE TABLE courses (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  language     text NOT NULL,
  cefr_level   text NOT NULL CHECK (cefr_level IN ('A1','A2','B1','B2','C1','C2')),
  title        text NOT NULL,
  description  text,
  version      int  NOT NULL DEFAULT 1,
  published    boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_courses_lang_level ON courses(language, cefr_level) WHERE published;

CREATE TABLE lessons (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id  uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  position   int  NOT NULL,
  title      text NOT NULL,
  version    int  NOT NULL DEFAULT 1,
  UNIQUE (course_id, position)
);

CREATE TABLE lesson_items (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lesson_id  uuid NOT NULL REFERENCES lessons(id) ON DELETE CASCADE,
  position   int  NOT NULL,
  kind       text NOT NULL CHECK (kind IN
              ('text','image','audio','video','exercise','speaking',
               'dialogue','mini_story','pov_story','story','podcast')),
  ref_id     uuid,                       -- points at the typed content row
  payload    jsonb NOT NULL DEFAULT '{}',
  media_key  text,
  UNIQUE (lesson_id, position)
);

CREATE TABLE exercises (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lesson_item_id uuid REFERENCES lesson_items(id) ON DELETE CASCADE,
  type           text NOT NULL CHECK (type IN
                  ('mcq','fill_blank','match','reorder','listen_choose',
                   'translate','speak','listen_answer')),
  prompt         jsonb NOT NULL,
  answer_key     jsonb NOT NULL,
  difficulty     smallint NOT NULL DEFAULT 1
);
CREATE INDEX idx_exercises_item ON exercises(lesson_item_id);
