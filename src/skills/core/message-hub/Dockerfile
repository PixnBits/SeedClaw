# ── message-hub ────────────────────────────────────────────────────────────
# Multi-stage build: golang:alpine builder → minimal alpine runtime
# SECURITY_INVARIANT: ARCHITECTURE.md §Message-Hub – sole IPC router
# SECURITY_INVARIANT: PRD.md §3.2 – TCP client only, no listening ports
# No EXPOSE – message-hub dials OUT to host.internal:7124 only.
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY messagehub.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /messagehub messagehub.go

FROM alpine:3.19
# nc (netcat) is used by the healthcheck in compose.yaml.
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /messagehub /usr/local/bin/messagehub
ENTRYPOINT ["/usr/local/bin/messagehub"]
