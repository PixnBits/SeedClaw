# ── user-agent ─────────────────────────────────────────────────────────────
# Multi-stage build: golang:alpine builder → minimal alpine runtime
# SECURITY_INVARIANT: ARCHITECTURE.md §Default Container Runtime Profile
# SECURITY_INVARIANT: user-agent/SKILL.md v2.1.2 – paranoid safety loop
# No EXPOSE – no ports published. All comms through message-hub only.
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY useragent.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /useragent useragent.go

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /useragent /usr/local/bin/useragent
ENTRYPOINT ["/usr/local/bin/useragent"]
