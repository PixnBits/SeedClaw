# Use a small static base — we want very little attack surface
FROM alpine:3.19

# Install minimal runtime deps + su-exec to drop privileges later
RUN apk add --no-cache ca-certificates su-exec

WORKDIR /app

# Copy source and build inside the image for an easy MVP
COPY messagehub.go /app/messagehub.go
RUN apk add --no-cache go \
    && go mod init seedclaw/message-hub \
    && go build -o /message-hub ./messagehub.go \
    && rm -rf /go/pkg /go/src \
    && apk del go

# Create a dedicated non-root user/group inside the image. We will run
# the container as root initially so the entrypoint can chown the mounted
# socket to this user:group, then drop privileges using su-exec.
RUN addgroup -g 1500 seedclaw \
    && adduser -D -u 10001 -G seedclaw -s /bin/sh seedclaw \
    && mkdir -p /run/sockets/control \
    && chown 10001:1500 /run/sockets/control \
    && chmod 0750 /run/sockets/control

# Copy entrypoint helper which will prepare the mounted socket and drop
# privileges to the non-root user for running the binary.
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/message-hub"]
