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
- **S3-compatible** (ArvanCloud, MinIO, AWS) — build with `-tags s3`
  (`go get github.com/minio/minio-go/v7` first), set `S3_*` env, and point
  `MEDIA_PUBLIC_URL` at the bucket/CDN base. Delivery goes straight from the
  bucket/CDN — no API fan-out.

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

A fresh database starts empty. `make seed` (and `docker compose up`, where the
migrate step runs `migrate seed`) loads:

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
