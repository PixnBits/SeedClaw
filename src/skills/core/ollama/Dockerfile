# Multi-stage build: build Go binary first
FROM golang:alpine AS builder

WORKDIR /app
COPY ollama.go /app/ollama.go

RUN go mod init seedclaw/ollama && go build -o /ollama ollama.go

# Use official Ollama image
FROM ollama/ollama:latest

# Copy the binary
COPY --from=builder /ollama /ollama

# Expose port
EXPOSE 11434

# Set user
USER 1000:1000

# Run the wrapper
CMD ["/ollama"]