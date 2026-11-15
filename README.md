# Leksuu Platform — Backend (Phase 0)

Go 1.26 modular monolith built on **Clean Architecture** (see `ARCHITECTURE.md`)
for the Leksuu language-learning platform. This is **Phase 0**: the verified data
foundation, a layered project structure, and a complete auth module.

## Status

| Piece | State |
|---|---|
| Module-oriented Clean Architecture (`shared/` + `modules/<ctx>/` + `platform/`) | ✅ — each feature is a self-contained vertical slice; `main.go` is 18 lines; all wiring in `platform/app` |
| Localized messages (no hardcoded user text; fa/ckb/en catalog) | ✅ — verified live: same error code returns Persian/English/Kurdish per `Accept-Language` |
| Config loader (12-factor env) + `/healthz` | ✅ |
| Full production schema (11 migrations, up + down) | ✅ **verified against PostgreSQL 16** — applies clean and fully reverses |
| Auth module (JWT + refresh rotation + reuse detection, RBAC) | ✅ **built, unit-tested, and verified live over HTTP** |
| Profile update (`PATCH /me`) + change password (`POST /me/password`) | ✅ **built, verified live** (wrong-current rejected, old password invalidated) |
| Course module: learning tree + progress; **server-side grading for mcq / fill-blank / reorder / match** with solution feedback; XP via reward port | ✅ **built, unit-tested, verified live**; answer keys never exposed |
| Gamification module (XP, streak, league, daily goal; awarded on learning actions) | ✅ **built, unit-tested, verified live** |
| Admin authoring module (RBAC CRUD: courses/lessons/items/exercises, any language) | ✅ **built, verified live**; non-admins get 403 |
| Demo content seed + admin bootstrap (`make seed`, `ADMIN_EMAILS`) | ✅ **seed verified on real PostgreSQL 16** |
| Storage selection by build tag (in-memory ↔ pgx Postgres) | ✅ |
| Frontend design system (Leksuu palette) | ⏳ next step |

### Auth module (`internal/modules/auth`)

Zero external dependencies in the default build, so it compiles and tests
anywhere. Security-critical pieces are hand-built on the standard library and
verified against published test vectors:

- **Password hashing** — PBKDF2-HMAC-SHA256 (210k iterations, random 16-byte
  salt, constant-time compare), encoded as `pbkdf2_sha256$iter$salt$hash`.
  Verified against the RFC PBKDF2 test vectors. Production upgrade path to
  argon2id is a drop-in (the encoded format absorbs it).
- **Access tokens** — HS256 JWT (stdlib), 15-min TTL, carrying `sub` + `roles`.
- **Refresh tokens** — opaque random strings; only their SHA-256 hash is stored.
  Rotation on every use; **reuse of a rotated token revokes the whole token
  family** (theft detection). 30-day TTL.
- **RBAC** — `Authenticator.Require` middleware + `Authenticator.RequireRole(role)`.
- **Storage** — domain `Repository` interfaces with an in-memory impl (dev/tests) and a
  pgx PostgreSQL impl behind `//go:build pgx` (its SQL is validated against the
  live schema). Enable with `go build -tags pgx` after `go mod tidy`.

Endpoints: `POST /api/v1/auth/{register,login,refresh,logout}`, `GET /api/v1/me`.
Run `go test ./...` to see the crypto vectors, rotation, and reuse-detection tests pass.

> Modules needing external deps (the pgx repo) compile on your machine via
> `go mod tidy`; they weren't fetchable in the build sandbox, so their SQL was
> validated directly against PostgreSQL instead.

## Run

### Option A — full stack in Docker (recommended)

From `backend/`:

```bash
docker compose up --build
```

This builds the API image (with the pgx adapter), starts **PostgreSQL** and
**Redis**, runs the **migrate** job (applies all migrations, then exits), and
finally starts the **api** on http://localhost:8080. Order is enforced with
health checks + `depends_on`, so the API never starts before the DB is migrated.

```bash
curl localhost:8080/healthz
curl -X POST localhost:8080/api/v1/auth/register \
  -d '{"Email":"me@leksuu.app","Password":"pw123456","DisplayName":"Me","NativeLang":"fa"}'
```

### Option B — app on host, infra in Docker (fast iteration)

```bash
docker compose up -d db redis        # just the infra
cp .env.example .env
go run -tags pgx ./cmd/migrate        # apply migrations (needs `go mod tidy` once)
go run ./cmd/api                      # in-memory repos, no DB needed; serves :8080
# or, against Postgres:  go run -tags pgx ./cmd/api
```

The **default build needs no database or dependencies at all** — `go run
./cmd/api` boots with in-memory repositories, which is the quickest way to try
the API. Add `-tags pgx` to switch to PostgreSQL.

### Build tags

| Command | Storage | Needs |
|---|---|---|
| `go run ./cmd/api` | in-memory | nothing |
| `go run -tags pgx ./cmd/api` | PostgreSQL | `go mod tidy`, a running DB |
| `go run -tags pgx ./cmd/migrate` | — | applies `migrations/*.up.sql` |

## How the four learning methods map to the schema

The pedagogy is baked into the data model so later phases don't need rework.

**AJ Hoge — Effortless English** (`0004_mini_stories_pov`)
- *Rule 1 (phrases, not words):* `vocabulary_items.item_kind` = `word | phrase | chunk | collocation`; SRS cards point at phrases too.
- *Rule 5 (Point-of-View stories):* `stories_base` + `pov_variants` — the same story stored in `present | past | present_perfect | future | past_perfect` so grammar is absorbed, never taught as rules.
- *Rule 7 (Listen-and-Answer):* `mini_stories` + `mini_story_questions` (q_type, `pause_ms`, `expected`, `accepts[]`) — the narrator asks, the learner answers aloud.
- *Rule 4 (deep repetition):* `deep_listen_log` tracks loop counts per content item.

**ELSA — pronunciation** (`0006_speaking_pronunciation`)
- Phoneme-level scoring in `pronunciation_scores.phoneme_scores`.
- `user_phoneme_profile` is the per-user "problem sounds" profile; `pronunciation_drills` are surfaced when a phoneme's `avg_score` is low.
- `phonemes.mouth_hint` carries ELSA-style mouth-shape guidance.

**Busuu — structured content + native community** (`0007_writing_ai_community`)
- CEFR-structured courses (`courses.cefr_level`).
- `community_submissions` + `community_feedback`: learners post writing/speaking; native speakers correct it.

**Media + graded stories — Zabanshenas-style** (`0005_podcasts_stories`)
- `media_segments` (shared by podcasts and stories) gives click-to-play + lazy, cached `translation`; clicking a word sends it to `srs_cards` (Leitner box).

Plus gamification (`user_gamification`, `daily_goals`, `achievements`) for the
Duolingo/Expert-style daily loop, and offline-first scaffolding (`offline_packages`,
`sync_state`, `sync_mutations` with idempotent `op_id`).


## Media & object storage

Lessons can carry images, audio, and video. Admins upload files via
`POST /api/v1/media` (multipart `file`, admin-only); the response is a public
URL stored in the lesson item's payload, and the player renders it by kind.

Storage is a port with two implementations, chosen at the composition root:

- **Local disk** (default) — files written under `MEDIA_DIR`, served at
  `GET /media/{key}` with `Cache-Control: public, max-age=31536000, immutable`
  and HTTP range support (so video seeks). Good for dev / single node.
- **S3-compatible** (ArvanCloud, MinIO, AWS) — the production Docker image
  includes the `s3` build tag. Set `S3_ENDPOINT`, `S3_REGION`, `S3_BUCKET`,
  `S3_PREFIX`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` and `S3_SECURE`. With
  `S3_PUBLIC_READ=false`, keep `MEDIA_PUBLIC_URL` at the site origin; the API
  serves the private bucket at the existing `/media/{key}` URLs. Public bucket
  delivery requires `S3_PUBLIC_READ=true` and a bucket/CDN base URL.

Before switching an existing local installation to S3, run the read-only
`go run -tags s3 ./cmd/media-migrate -manifest media-inventory.json` with the
old `MEDIA_DIR` mounted and the destination S3 settings. Review the checksums
and keys, then run with `-apply` to copy and verify. The tool refuses to
overwrite a different object and never removes local files. Keep the private
delivery mode and the same site origin to preserve existing database URLs.

Admin story cover generation uses `COVER_IMAGE_API_KEY` and optional
`COVER_IMAGE_MODEL` (default `gpt-image-2.5-flare`). The key stays server-side.
The endpoint is admin-only and generated images require approval before the
story payload changes.

### Automatic media preparation

The media worker discovers saved unpublished Music, Film, Podcast and audio
Story drafts automatically. It runs one stage at a time for each draft: HLS
for Film/Music, full-recording transcription/alignment, a second speech-model
check, Persian translation and Sorani translation. Results are saved to the
draft automatically. No per-line acceptance is needed for checks that pass.
Only flagged passages need attention; publication remains a final admin action.
The worker never publishes or confirms rights. Existing published content is
excluded from automatic discovery.

Automatic acceptance is recorded as `auto_approved`, separately from human
`approved`. The second recognizer must match every source word, agree on cue
boundaries within 0.75 seconds, and report word confidence of at least 0.85.
Alignment exceptions override acceptance. Translation checks use script and
English round-trip similarity (at least 0.85); they are conservative heuristics,
not proof of semantic accuracy or native fluency. Non-English source translation
checks remain exceptions. Model disagreement, music/silence gaps and suspicious
translations are shown in the admin exception list. No model can guarantee
perfect recognition, especially for singing or overlapping dialogue.

Deployment requires the updated API, frontend and worker image. Run the worker
against the intended existing database with migrations through 0062 already
applied. Do not start a default Compose database as a substitute for existing
content. No new schema migration is required for this workflow. Queue polling,
leases, bounded retries, cancellation and checkpoints survive page closure.
A failed stage stays failed until retried; it is not continuously requeued.
Edits made while processing cause a conflict instead of being overwritten.
Transient provider timeouts or connection failures are requeued with a short
database-backed delay and stop after the third attempt. Validation failures,
missing models, and source conflicts stay failed for targeted attention.

`MEDIA_JOBS_AUTOMATION_ENABLED=true` is the worker default; set it to `false`
to retain the older manual workflow. Default recognition uses `small.en` and
verification uses `medium.en` for English; other languages default to the
multilingual `small` and `medium` models. Leave `MEDIA_JOBS_WHISPER_MODEL` and
`MEDIA_JOBS_VERIFICATION_MODEL` empty for language-aware defaults, or override
them with two different appropriate models. The second pass increases CPU/memory use and latency.
Set `MEDIA_JOBS_LANGUAGE_DETECTION_MODEL` to a locally available multilingual
Whisper model. A missing detector or mismatch with the declared source language
keeps machine source review unresolved; it does not guess a new language or
invent text. The worker still stores draft transcript evidence for correction.
`MEDIA_JOBS_RECONCILE_SECONDS` defaults to 30. Prepare both speech models in the
persistent model volume and install the MADLAD translation model using
`worker/translation_model_setup.py` before processing real uploads. Missing
model/dependency errors remain visible instead of passing checks.
When a draft has verified rights and a reference, the worker can extract a
supported embedded text subtitle track from Film or Podcast media and compare
it with the actual audio before accepting timing. The track remains draft
evidence if recognition or language checks disagree. Existing supplied cues
take precedence and are not replaced by embedded subtitles.

New Film, Music, and Podcast uploads can create a queued draft immediately after
the file upload. Automatic publication is disabled by default with
`MEDIA_JOBS_AUTO_PUBLICATION_ENABLED=false`. It also requires the item's explicit
`auto_publish` choice, verified rights and a rights reference, a passing second
speech-recognizer check, current per-cue timing, complete Persian and Sorani
translations, and HLS for Film/Music. Configure a **distinct** local CTranslate2
model at `MEDIA_JOBS_TRANSLATION_VERIFICATION_MODEL_DIR` for the backtranslation
check; it must support English as a target and must not be the production
translation model directory. Without it, generated translations stay drafts.
Machine approval additionally requires `MEDIA_JOBS_TRANSLATION_REFERENCE_FILE`,
a read-only JSON file with `version: 1`,
`provenance: "independent_editorial_reference"`, and at least 12 distinct
English `source` cases each containing independently prepared `fa` and `ckb`
references. The worker evaluates the configured model against each language's
references; missing, weak, or malformed evidence leaves translations as drafts.
`MEDIA_JOBS_TRANSLATION_CUE_ATTEMPTS` bounds targeted regeneration to 1–3
attempts (default 2), preserving human and imported drafts. Roundtrip/script
and reference checks cannot guarantee idioms or Sorani fluency, so enable
publication only after broader staging evaluation of the selected models.
`MEDIA_JOBS_TRANSLATION_INTRA_THREADS` defaults to 1 and is clamped to 1–4;
larger values can need substantially more RAM while loading MADLAD on CPU.
Failed checks leave a recorded unpublished
exception. No worker infers media publication rights from a file upload.

`MEDIA_JOBS_TRANSLATION_PROVIDER` defaults to `local`. Optional `google` and
`azure` draft providers require `MEDIA_JOBS_CLOUD_TRANSLATION_ALLOWED=true`, a
positive `MEDIA_JOBS_CLOUD_MAX_CHARS_PER_JOB`, explicit credentials, and item
rights approval/reference. Google uses `ckb`; Azure uses its documented Central
Kurdish `ku` text-translation code. The generic `ku` detector label never
establishes Sorani source identity. Cloud responses remain drafts because
their independent quality calibration has not been established. Each request
is limited to 4,000 characters with a 15-second timeout and configurable
minimum spacing; transient failures use the existing bounded job retry.
`MEDIA_JOBS_TRANSLATION_MACHINE_APPROVAL_ENABLED` defaults to `false` while
Phase 2D quality thresholds lack an independently reviewed domain benchmark.
Generated text remains a draft even when other technical checks succeed.

Set `MEDIA_PUBLIC_URL` to the public site origin so the worker can map stored
URLs to `http://api:8080`; external media origins are rejected. The `media-models`
volume keeps downloaded models between restarts. `MEDIA_JOBS_MAX_BYTES` caps
an upload at 2 GiB by default. Translation is limited to 2,000 cues and 100,000
characters by default (`MEDIA_TRANSLATION_MAX_CUES` / `MEDIA_TRANSLATION_MAX_CHARS`).
Use local CPU inference; no paid transcription/translation API is invoked.
`MEDIA_JOBS_DATABASE_URL` overrides `DATABASE_URL`. The worker image uses Python
3.11 with PyAV; the first model download needs network access. Compose's
`media-processing` profile supplies the worker, but its default database target
must be reviewed before running it in an existing environment.

**Load-speed choices:** content-addressed keys + a one-year immutable cache
(instant repeat loads / CDN-friendly), HTTP range requests for video, and on the
client: images `loading="lazy"` + `decoding="async"`, video `preload="none"`
with a poster (no download until play), audio `preload="metadata"` (duration
only).

## Migrations

11 paired up/down files, `golang-migrate` naming. Verified: full up → clean;
full down → 0 tables. Three tables are `PARTITION BY RANGE (created_at)`
(`user_progress`, `speaking_sessions`, `pronunciation_scores`) with DEFAULT
partitions; the worker will create monthly partitions ahead of time.

## Next steps

1. **Frontend design system** — Next.js + Tailwind tokens in the Leksuu palette
   (brown #5A3E2B, beige #E7D6C1, cream #FDFAF6, ink #141008), RTL, shared components.
2. Then Phase 1 (core learning loop + SRS + gamification) per the roadmap.


## Seeding & admin access

A fresh database starts empty. `make seed` or
`docker compose run --rm migrate migrate seed` explicitly loads the demo seed
data into a disposable database. Normal `docker compose up` applies migrations
without seeding existing records. The seed loads:

- a starter **English A1** course, and
- a default **admin account** so the panel works immediately.

**Default admin login**

    email:    admin@leksuu.app
    password: leksuu-admin

Sign in at `/[locale]/login`, then the **Admin** link appears in the header
(only for admins) → open the panel to author courses, lessons, and the four
interactive exercise types, in any teaching language. Change the password after
first login.

This also works in **memory mode** (no database): on startup the server ensures
each `ADMIN_EMAILS` account exists, creating it with `ADMIN_PASSWORD` (default
`leksuu-admin`) if missing — so `admin@leksuu.app` / `leksuu-admin` logs in and
gets the admin role with no registration and no seed. The admin link in the
header is hidden for everyone else.
