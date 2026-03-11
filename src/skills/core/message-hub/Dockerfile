FROM alpine:3.19 AS builder

RUN apk add --no-cache go

WORKDIR /app

COPY messagehub.go /app/messagehub.go

RUN go mod init seedclaw/message-hub \
    && go build -o /message-hub ./messagehub.go

# Use a small static base — we want very little attack surface
FROM alpine:3.19 AS skill
# Install only ca-certificates (for future https if needed) + tzdata if you want logging with time
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /message-hub /app/message-hub

# Drop to non-root
USER 10001:10001

CMD ["/app/message-hub"]
