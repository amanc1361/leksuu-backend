-- 0010 — offline-first: versioned content packages + per-device sync cursors.
CREATE TABLE offline_packages (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  course_id       uuid NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
  content_version int NOT NULL,
  manifest_key    text NOT NULL,                -- JSON manifest of {key,hash,bytes}
  bytes           bigint NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE (course_id, content_version)
);

CREATE TABLE sync_state (
  user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id      text NOT NULL,
  last_pulled_at timestamptz NOT NULL DEFAULT 'epoch',
  last_pushed_seq bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, device_id)
);

-- Idempotent progress mutations pushed from offline clients (op_id dedupe).
CREATE TABLE sync_mutations (
  op_id      uuid PRIMARY KEY,                  -- client-generated; dedupes retries
  user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  device_id  text NOT NULL,
  op         text NOT NULL,
  payload    jsonb NOT NULL,
  client_ts  timestamptz NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_sync_mut_user ON sync_mutations(user_id, applied_at);
