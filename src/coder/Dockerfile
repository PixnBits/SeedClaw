FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY coder.go .
RUN go build -o coder coder.go

FROM alpine:latest
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/coder /app/coder
CMD ["/app/coder"]