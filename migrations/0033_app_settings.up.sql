-- Platform settings as a single JSON row, editable from the admin console.
CREATE TABLE IF NOT EXISTS app_settings (
    id   int PRIMARY KEY,
    data jsonb NOT NULL
);
