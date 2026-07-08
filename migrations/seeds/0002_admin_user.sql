-- Default admin so the panel is usable out of the box. Idempotent.
--   email:    admin@leksuu.app
--   password: leksuu-admin
-- Change the password after first login. (If this email self-registers instead,
-- the ADMIN_EMAILS bootstrap still grants the admin role on login.)
BEGIN;

INSERT INTO users (id, email, password_hash, display_name, native_lang, target_lang, cefr_level, status)
VALUES (
  '00000000-0000-0000-0000-0000000000a0',
  'admin@leksuu.app',
  'pbkdf2_sha256$210000$WN6QkEQbNCiJlw5Ze7s7mw$5sHcPxjlweQT5rHhcKYfFjQHSvdpmxjL8HRTTjDl70U',
  'Leksuu Admin', 'fa', 'en', 'A1', 'active'
)
ON CONFLICT (email) DO NOTHING;

-- grant the admin role (role_id 3 = admin) to whichever row holds that email
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, 3 FROM users u WHERE u.email = 'admin@leksuu.app'
ON CONFLICT (user_id, role_id) DO NOTHING;

COMMIT;
