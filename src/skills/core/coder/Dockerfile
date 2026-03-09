# Use a small static base
FROM alpine:3.19

# Install ca-certificates for HTTPS
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Copy the source
COPY coder.go /app/coder.go

# Build inside
RUN apk add --no-cache go \
    && go mod init seedclaw/coder \
    && go build -o /coder ./coder.go \
    && rm -rf /go/pkg /go/src

# Drop to non-root
USER 10001:10001