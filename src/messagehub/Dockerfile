FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY messagehub.go .
RUN go build -o messagehub messagehub.go

FROM alpine:latest
COPY --from=builder /app/messagehub /app/messagehub
CMD ["/app/messagehub"]