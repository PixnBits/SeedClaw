# ── ollama ─────────────────────────────────────────────────────────────────
# CRITICAL: This is a DUAL-PROCESS image.
# Stage 1: build the Go supervisor binary.
# Stage 2: copy the Go binary into the official ollama/ollama image.
# The Go binary is ENTRYPOINT / PID 1. It starts `ollama serve` as a child.
#
# SECURITY_INVARIANT: ollama/SKILL.md v2.1.4 – Go binary = PID 1
# SECURITY_INVARIANT: No EXPOSE 11434 – port is internal only (127.0.0.1).
# SECURITY_INVARIANT: ollama HTTP API must bind to 127.0.0.1 only.
FROM golang:1.22-alpine AS builder

WORKDIR /build
COPY go.mod ./
COPY ollama.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /ollama-skill ollama.go

# Stage 2: official Ollama image with our Go binary overlaid
FROM ollama/ollama:latest

# Copy the compiled supervisor binary.
COPY --from=builder /ollama-skill /usr/local/bin/ollama-skill

# MUST be our binary, not `ollama serve`.
# SECURITY_INVARIANT: ollama/SKILL.md v2.1.4 – Go binary = PID 1
ENTRYPOINT ["/usr/local/bin/ollama-skill"]
# CMD is intentionally omitted – all args handled by the Go supervisor.
