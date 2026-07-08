-- 0001 — extensions, identity, roles, sessions
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext UNIQUE NOT NULL,
  password_hash text,
  display_name  text NOT NULL,
  native_lang   text NOT NULL DEFAULT 'fa',          -- 'fa','ckb',...
  target_lang   text NOT NULL DEFAULT 'en',
  cefr_level    text NOT NULL DEFAULT 'A1'
                CHECK (cefr_level IN ('A1','A2','B1','B2','C1','C2')),
  status        text NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','suspended','deleted')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE roles (
  id   smallint PRIMARY KEY,
  name text UNIQUE NOT NULL
);
INSERT INTO roles (id, name) VALUES (1,'student'),(2,'teacher'),(3,'admin');

CREATE TABLE user_roles (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id smallint NOT NULL REFERENCES roles(id),
  PRIMARY KEY (user_id, role_id)
);

CREATE TABLE refresh_tokens (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  text NOT NULL,
  family_id   uuid NOT NULL,
  device_id   text,
  expires_at  timestamptz NOT NULL,
  revoked_at  timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_user ON refresh_tokens(user_id) WHERE revoked_at IS NULL;
