# API image (specs/containerization/). Versions are reviewed on purpose - see
# requirements.md §6 there before raising them.

# Build stage: same Go line as go.mod and CI (CLAUDE.md §2).
FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first: this layer is reused until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Only the Go source; .dockerignore keeps everything else out of the build context.
COPY cmd ./cmd
COPY internal ./internal
ENV CGO_ENABLED=0
# /bootstrap-admin creates the first admin user on a freshly migrated database
# (specs/database-migrations/ BR10), run as a one-off command/Job with this image.
RUN go build -o /out/api ./cmd/api \
    && go build -o /out/bootstrap-admin ./cmd/bootstrap-admin

# Runtime stage: only the two binaries on a pinned Alpine minor.
FROM alpine:3.24
# Numeric UID/GID so Kubernetes runAsNonRoot can verify the user. The binaries
# stay owned by root: executable, not writable, by this user.
RUN addgroup -S -g 10001 app \
    && adduser -S -D -H -u 10001 -G app -s /sbin/nologin app
COPY --from=build /out/api /api
COPY --from=build /out/bootstrap-admin /bootstrap-admin
USER 10001:10001
EXPOSE 8080
# BusyBox wget (already in Alpine) fails on a non-2xx response, a refused
# connection or a timeout. Kubernetes ignores this and uses its own probes.
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -T 2 -O /dev/null "http://127.0.0.1:${PORT:-8080}/health" || exit 1
CMD ["/api"]
