# ── llm-caller ─────────────────────────────────────────────────────────────
# Multi-stage build: golang:alpine builder → minimal alpine runtime
# SECURITY_INVARIANT: ARCHITECTURE.md §Default Container Runtime Profile
# SECURITY_INVARIANT: PRD.md §4.2 – read_only, cap_drop ALL, seedclaw-net only
# No EXPOSE – no ports are published. All comms through message-hub only.
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY llmcaller.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /llmcaller llmcaller.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /llmcaller /usr/local/bin/llmcaller
ENTRYPOINT ["/usr/local/bin/llmcaller"]
