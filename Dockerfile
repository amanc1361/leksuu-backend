# ---- build stage: compile the API and migrator with the PostgreSQL adapter ----
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache git
# GOPROXY is overridable for restricted networks (e.g. an Iranian Go proxy):
#   docker compose build --build-arg GOPROXY=https://goproxy.cn,direct
ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
COPY . .
# Production includes the pgx and S3 adapters. If S3_BUCKET is empty the app
# still falls back to local disk, so the same image works in development too.
# Resolving dependencies here also
# generates go.sum on first build (the default build is dependency-free).
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -tags pgx,s3 -ldflags="-s -w" -o /out/api ./cmd/api && \
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
