# Backend Architecture

A **modular monolith** in Go, organized by **bounded context** with Clean
Architecture *inside each module*. Dependencies point inward only. This layout is
chosen specifically so the codebase stays clean as features multiply: each new
capability is a self-contained vertical slice, never another file dumped into a
shared `domain` or `handlers` folder.

```
cmd/
  api/main.go          # 18 lines: load config, build app, run
  migrate/             # SQL migration runner (build tag pgx) + no-pgx stub
internal/
  shared/              # SHARED KERNEL — cross-cutting, no business rules
    apperr/            # Error type + Kind (the error vocabulary every module uses)
    id/                # random identifiers
    authn/             # authenticated principal carried in context (cross-module)
    message/           # localized message catalog (fa/ckb/en) + negotiation
    web/               # generic HTTP: JSON + error->status+localized-message
  modules/             # ONE FOLDER PER BOUNDED CONTEXT
    auth/              # the authentication module (vertical slice)
      domain/          #   entities, repository ports, sentinel errors
      usecase/         #   application business rules + the ports it needs
      security/        #   infra adapters: PBKDF2 hasher, HS256 JWT, refresh tokens
      repo/            #   persistence adapters: memory + postgres (tag pgx)
      rest/            #   delivery adapters: handlers, routes, auth middleware
      module.go        #   facade: New(deps) -> wires this module; Mount; Authenticator
    course/            # learning module (courses -> lessons -> items -> exercises + progress)
      domain/ usecase/ repo/ rest/ module.go   # same shape; reuses auth via a Guard; grants XP via a Rewarder port
    gamification/      # motivation module (XP, streak, league, daily goal)
      domain/ usecase/ repo/ rest/ module.go   # exposes Rewards() — the award entry point
  platform/            # PROCESS / RUNTIME concerns + composition root
    config/            # env loading (12-factor)
    httpserver/        # reusable net/http lifecycle wrapper
    app/               # composition root: wire modules, mount routes, run
                       # repos.go / repos_pgx.go select storage by build tag
```

## Dependency rule

```
platform/app  ──build&wire──▶  modules/*  ──▶  shared/*
                                  │
              domain ◀── usecase ◀── security / repo / rest   (inward only)
```

- **domain** imports only `shared/apperr`. It knows no transport, no database.
- **usecase** imports its module's `domain` (+ `shared/id`) and declares the
  *ports* it needs (`Hasher`, `AccessTokenIssuer`, …). It depends on no adapter.
- **security / repo / rest** are adapters that *implement* those ports / domain
  interfaces. They depend inward on `usecase` / `domain`, never the reverse.
- **module.go** is the module's own little composition root; it wires the slice
  from injected repositories and exposes a tiny surface (`Mount`, `Authenticator`).
- **platform/app** is the *application* composition root: it builds repositories
  (memory or pgx by build tag), constructs each module, and mounts routes. It is
  the only place that knows every concrete type.

## Two rules that keep it clean as it grows

1. **`main.go` stays trivial.** It asks `app.New` for a wired application and runs
   it. Every wiring decision lives in `platform/app`.
2. **No user-facing prose in code.** Business and HTTP code reference stable
   message *codes* (`auth.email_taken`). `shared/message` maps each code to
   fa/ckb/en, negotiated per request from `Accept-Language`.

## Cross-module auth

A module that needs the caller's identity does not depend on the auth module's
internals. The auth middleware publishes a `shared/authn.Principal` into the
request context; any handler reads it with `authn.FromContext`. Protected routes
are wrapped by a small `Guard` interface (satisfied by the auth module's
`Authenticator`), injected at wiring time — e.g. the course module guards its
progress endpoints without importing auth.

The same pattern carries rewards the other direction: the course module declares
a `Rewarder` port and calls it when a lesson is completed or an exercise answered
correctly. The gamification module's award service satisfies that port
(structural typing), and the composition root wires it in — so course grants XP
without importing gamification, and gamification never imports course.

## How to add a new module (the `course` module is the worked example)

1. `internal/modules/course/domain` — entities, repository ports, error codes.
2. `internal/shared/message` — localized text for those codes.
3. `internal/modules/course/usecase` — use cases + the ports they need.
4. `internal/modules/course/repo` — `memory.go` + `postgres.go` (tag pgx).
5. `internal/modules/course/rest` — handlers + `Mount`.
6. `internal/modules/course/module.go` — `New(deps)` wiring + `Mount`.
7. `internal/platform/app` — build the module's repos and `courseModule.Mount(mux, apiPrefix)`.

`cmd/api/main.go` never changes. No shared folder turns into a grab-bag, because
each context owns its own `domain` / `usecase` / adapters.

## Storage selection (no change to the composition logic)

`buildRepos` is chosen at compile time:
- default build → `platform/app/repos.go` → in-memory repositories.
- `go build -tags pgx` → `platform/app/repos_pgx.go` → PostgreSQL via pgx.

Dev and tests run with zero infrastructure; production flips on with a build tag.
