# Build from the OpenFinCore root, so the sibling permissions module (see the
# `replace` in go.mod) is part of the build context:
#
#   docker build -f user-service/dockerfile -t ofc-user-service .
#
# The image runs in the production stage. Secrets come from the environment
# (the service refuses to start without them outside development):
#
#   OFC_OTP_HASH_SECRET    >= 32 chars, e.g. `openssl rand -hex 32`
#   OFC_DB_PASSWORD        password of the app role (postgreDataSource.user)
#   OFC_DB_ADMIN_PASSWORD  admin/migration role, only when auto-migrating
#   OFC_DB_HOST / OFC_REDIS_ADDR / OFC_REDIS_PASSWORD / OFC_HTTP_TRUSTED_PROXIES ...
#
# See conf/config.go (applyEnv) for every override.

# ---- build ----------------------------------------------------------------
FROM golang:1.25-bookworm AS builder

WORKDIR /src
# Dependencies first, so source edits don't invalidate the module cache.
COPY permissions/ ./permissions/
COPY user-service/go.mod user-service/go.sum ./user-service/
WORKDIR /src/user-service
RUN go mod download

COPY user-service/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ofc-user-service . \
    && mkdir -p /out/logger/logs

# ---- run ------------------------------------------------------------------
# Distroless static: no shell or package manager, runs as an unprivileged
# user (uid 65532).
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/ofc-user-service ./ofc-user-service
COPY --from=builder --chown=nonroot:nonroot /out/logger ./logger
COPY user-service/conf/config.json ./conf/config.json
COPY user-service/seeds ./seeds

# Overrides "stage" in config.json: an image can never start in development
# mode (OTP in responses, dev tenant) unless explicitly asked to.
ENV OFC_STAGE=production

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/ofc-user-service"]
