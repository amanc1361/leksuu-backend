-- External dictionary providers: admin-configured lookup APIs (word -> meaning).
CREATE TABLE dict_providers (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name          text NOT NULL,
  url           text NOT NULL,
  method        text NOT NULL DEFAULT 'GET',
  header_name   text,
  header_value  text,
  body_template text,
  response_path text,
  enabled       boolean NOT NULL DEFAULT true,
  position      int NOT NULL DEFAULT 0,
  created_at    timestamptz NOT NULL DEFAULT now()
);
