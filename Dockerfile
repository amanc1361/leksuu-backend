# ---- build stage: compile the API and migrator with the PostgreSQL adapter ----
FROM golang:1.26-alpine AS build
# ARG GOPROXY=https://proxy.golang.org,direct
# ENV GOPROXY=${GOPROXY}
WORKDIR /src
RUN apk add --no-cache git
# GOPROXY is overridable for restricted networks (e.g. an Iranian Go proxy):
#   docker compose build --build-arg GOPROXY=https://goproxy.cn,direct
COPY . .
# Production uses the pgx adapter (build tag `pgx`). Resolving it here also
# generates go.sum on first build (the default build is dependency-free).
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -tags pgx -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -tags pgx -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---- runtime stage: minimal, non-root ----
FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api /out/migrate /usr/local/bin/
COPY migrations ./migrations
USER app
EXPOSE 8080
# Both binaries are on PATH; compose overrides this for the one-shot migrate job.
CMD ["api"]
